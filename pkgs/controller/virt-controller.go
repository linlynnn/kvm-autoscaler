package controller

import (
	"context"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	genconfig "github.com/linlynnn/kvm-autoscaler/pkgs/gen-config"
	"github.com/linlynnn/kvm-autoscaler/pkgs/instance"
	"github.com/linlynnn/kvm-autoscaler/pkgs/lb"
	"libvirt.org/go/libvirt"
)

type VirtController struct {
	sync.Mutex
	conn                    *libvirt.Connect
	MapInstanceIdToInstance map[string]instance.InstanceManager
	LastScaleUp             time.Time
	LastScaleDown           time.Time
	ScaleUpCoolDown         time.Duration
	ScaleDownCoolDown       time.Duration
	loadBalancer            *lb.LoadBalancer
}

func NewVirtController(
	conn *libvirt.Connect,
	scaleUpCoolDown time.Duration,
	scaleDownCoolDown time.Duration,
	loadBalancer *lb.LoadBalancer,
) *VirtController {

	now := time.Now()
	lastScaleUp := now.Add(-scaleUpCoolDown - (1 * time.Second))
	lastScaleDown := now.Add(-scaleDownCoolDown - (1 * time.Second))

	return &VirtController{
		conn:                    conn,
		MapInstanceIdToInstance: make(map[string]instance.InstanceManager),
		LastScaleUp:             lastScaleUp,
		LastScaleDown:           lastScaleDown,
		ScaleUpCoolDown:         scaleUpCoolDown,
		ScaleDownCoolDown:       scaleDownCoolDown,
		loadBalancer:            loadBalancer,
	}

}

func (m *VirtController) ScaleUp(numToAdd int) {
	m.Lock()
	defer m.Unlock()

	var wg sync.WaitGroup
	//ctx, cancel := context.WithTimeout(context.Background(), m.ScaleUpCoolDown)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now()

	if now.Sub(m.LastScaleUp) < m.ScaleUpCoolDown {
		// ScaleUp is cooldown
		log.Println("ScaleUp is cooldown")
		return
	}

	log.Printf("Start ScaleUp %d\n", numToAdd)
	m.LastScaleUp = now
	m.LastScaleDown = now
	// ScaleUp logic
	for i := 0; i < numToAdd; i++ {
		// can be concurrent
		wg.Add(1)
		go m.createVM(ctx, &wg)
	}

	wg.Wait()

}

func (m *VirtController) ScaleDown(instancesToRemove []instance.InstanceManager) {
	m.Lock()
	defer m.Unlock()
	now := time.Now()

	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	if now.Sub(m.LastScaleDown) < m.ScaleDownCoolDown {
		// ScaleDown is cooldown
		log.Println("ScaleDown is cooldown")
		return
	}

	// ScaleDown logic
	log.Println("Start ScaleDown")
	m.LastScaleDown = now
	for _, instance := range instancesToRemove {
		// can be concurrent

		wg.Add(1)
		go m.gracefullyShutdown(instance, ctx, &wg)

	}
	wg.Wait()

}

func (m *VirtController) createVM(ctx context.Context, wg *sync.WaitGroup) error {
	defer wg.Done()

	done := make(chan error, 1)

	go func() {
		uuid := uuid.New()
		log.Printf("Creating VM instance-%v\n", uuid.String())
		if err := genconfig.GenQcow2DiskImage(uuid.String()); err != nil {
			log.Println(err)
			done <- err
			return
		}

		if err := genconfig.GenMetaDataInstanceConfig(uuid.String()); err != nil {
			log.Println(err)
			done <- err
			return
		}

		if err := genconfig.GenUserDataInstanceConfig(uuid.String(), os.Getenv("SSH_PUBLIC_KEY")); err != nil {
			log.Println(err)
			done <- err
			return
		}

		if err := genconfig.GenCdRomDiskImage(uuid.String()); err != nil {
			log.Println(err)
			done <- err
			return
		}

		virtInstanceConfigPath, err := genconfig.GenVirtInstanceConfig(uuid.String())
		if err != nil {
			log.Println(err)
			done <- err
			return
		}

		// read xml file and then register
		xmlBytes, err := os.ReadFile(virtInstanceConfigPath)
		if err != nil {
			log.Fatalf("Failed to read XML file: %v", err)
			done <- err
		}

		domainXML := string(xmlBytes)

		domain, err := m.conn.DomainDefineXML(domainXML)
		if err != nil {
			log.Fatalf("Failed to define domain: %v", err)
			done <- err
			return
		}

		instanceId := "instance-" + uuid.String()
		instanceMng := instance.NewVirtInstanceManager(domain, instanceId)

		m.Lock()
		m.MapInstanceIdToInstance[instanceId] = instanceMng
		m.Unlock()

		if err := domain.Create(); err != nil {
			done <- err
			return
		}

		if m.loadBalancer != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()
				instanceMng.RegisterIP(os.Getenv("LOAD_BALANCER_URL"), ctx)
			}()
		}

		log.Printf("Created VM instance-%v\n", uuid.String())
		done <- nil

	}()

	select {
	case <-ctx.Done():
		log.Println("createVM timeout/err", ctx.Err())
		return ctx.Err()
	case err := <-done:
		if err != nil {
			log.Println(err)
			return err
		}

	}

	return nil

}

func (m *VirtController) gracefullyShutdown(inst instance.InstanceManager, ctx context.Context, wg *sync.WaitGroup) error {
	defer wg.Done()
	// need to ensure that instance must shut off
	done := make(chan error, 1)

	go func() {

		// deregisterIP
		if m.loadBalancer != nil {

			loadBalancerUrl := os.Getenv("LOAD_BALANCER_URL")
			if loadBalancerUrl == "" {
				log.Println("LOAD_BALANCER_URL is not defined")
				return
			}

			inst.DeRegisterIP(loadBalancerUrl)

			drainingTime := os.Getenv("DRAINING_TIME_SEC")
			if drainingTime == "" {
				log.Println("DRAINING_TIME is not defined")
				return
			}

			drainingTimeInt, err := strconv.Atoi(drainingTime)
			if err != nil {
				log.Println(err)
				return
			}

			log.Printf("Draining connection %s\n", inst.GetID())
			time.Sleep(time.Duration(drainingTimeInt) * time.Second)

		}

		err := inst.Shutdown()
		if err != nil {
			done <- err
		}

		m.Lock()
		delete(m.MapInstanceIdToInstance, inst.GetID())
		m.Unlock()

		done <- nil

	}()

	select {
	case <-ctx.Done():
		log.Println("GracefullyShutdown context cancel %s\n", inst.GetID())
		return ctx.Err()

	case err := <-done:
		if err != nil {
			log.Println(err)
			return err
		}

	}

	return nil

}

func (m *VirtController) GetRunningInstance() (int, []instance.InstanceManager, error) {
	runningInstances := []instance.InstanceManager{}

	m.Lock()
	for _, instanceMng := range m.MapInstanceIdToInstance {
		instanceStatus := instanceMng.GetStatus()

		if instanceStatus == instance.VM_STATE_RUNNING {
			runningInstances = append(runningInstances, instanceMng)
		}

	}
	m.Unlock()

	return len(runningInstances), runningInstances, nil

}

func (m *VirtController) Close() {
	log.Println("Closing virt connection")
	m.conn.Close()
	log.Println("Closed virt connection")

}

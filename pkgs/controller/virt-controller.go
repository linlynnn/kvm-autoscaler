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

	now := time.Now()

	m.Lock()
	if now.Sub(m.LastScaleUp) < m.ScaleUpCoolDown {
		log.Println("[VirtController] ScaleUp is cooldown, last action", m.LastScaleUp)
		m.Unlock()
		return
	}
	m.Unlock()

	log.Printf("[VirtController] Start ScaleUp %d\n", numToAdd)
	m.Lock()
	m.LastScaleUp = now
	m.LastScaleDown = now
	m.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < numToAdd; i++ {
		wg.Add(1)
		go m.createVM(&wg)
	}

	wg.Wait()

}

func (m *VirtController) ScaleDown(instancesToRemove []instance.InstanceManager) {
	now := time.Now()

	m.Lock()
	if now.Sub(m.LastScaleDown) < m.ScaleDownCoolDown {
		log.Println("[VirtController] ScaleDown is cooldown, last action", m.LastScaleDown)
		m.Unlock()
		return
	}
	m.Unlock()

	log.Printf("[VirtController] Start ScaleDown %d\n", len(instancesToRemove))
	m.Lock()
	m.LastScaleDown = now
	m.Unlock()

	var wg sync.WaitGroup
	for _, instance := range instancesToRemove {
		wg.Add(1)
		go m.gracefullyShutdown(instance, &wg)

	}
	wg.Wait()

}

func (m *VirtController) createVM(wg *sync.WaitGroup) error {

	defer wg.Done()
	uuid := uuid.New()
	log.Printf("[VirtController] Creating VM instance-%v\n", uuid.String())
	if err := genconfig.GenQcow2DiskImage(uuid.String()); err != nil {
		log.Println(err)
		return err
	}

	if err := genconfig.GenMetaDataInstanceConfig(uuid.String()); err != nil {
		log.Println(err)
		return err
	}

	if err := genconfig.GenUserDataInstanceConfig(uuid.String(), os.Getenv("SSH_PUBLIC_KEY")); err != nil {
		log.Println(err)
		return err
	}

	if err := genconfig.GenCdRomDiskImage(uuid.String()); err != nil {
		log.Println(err)
		return err
	}

	virtInstanceConfigPath, err := genconfig.GenVirtInstanceConfig(uuid.String())
	if err != nil {
		log.Println(err)
		return err
	}

	// read xml file and then register
	xmlBytes, err := os.ReadFile(virtInstanceConfigPath)
	if err != nil {
		log.Printf("[VirtController] Failed to read XML file: %v\n", err)
	}

	domainXML := string(xmlBytes)

	domain, err := m.conn.DomainDefineXML(domainXML)
	if err != nil {
		log.Printf("[VirtController] Failed to define domain: %v\n", err)
		return err
	}

	instanceId := "instance-" + uuid.String()
	instanceMng := instance.NewVirtInstanceManager(domain, instanceId)

	m.Lock()
	m.MapInstanceIdToInstance[instanceId] = instanceMng
	m.Unlock()

	if err := domain.Create(); err != nil {
		log.Println(err)
	}

	if m.loadBalancer != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			instanceMng.RegisterIP(os.Getenv("LOAD_BALANCER_URL"), ctx)
		}()
	}

	go func() {
		instanceMng.RegisterPromDiscovery()
	}()

	log.Printf("[VirtController] Created VM instance-%v\n", uuid.String())
	return nil

}

func (m *VirtController) gracefullyShutdown(inst instance.InstanceManager, wg *sync.WaitGroup) error {
	defer wg.Done()
	// need to ensure that instance must shut off
	// deregisterIP
	if m.loadBalancer != nil {

		go func() {
			loadBalancerUrlEnv := os.Getenv("LOAD_BALANCER_URL")
			if loadBalancerUrlEnv == "" {
				log.Println("[VirtController] LOAD_BALANCER_URL is not defined, use fallback value: http://localhost:8080")
				loadBalancerUrlEnv = "http://localhost:8080"
			}

			inst.DeRegisterIP(loadBalancerUrlEnv)

			drainingTimeEnv := os.Getenv("DRAINING_TIME_SEC")
			if drainingTimeEnv == "" {
				log.Println("[VirtController] DRAINING_TIME is not defined, use fallback value: 30")
				drainingTimeEnv = "30"
			}

			drainingTime, err := strconv.Atoi(drainingTimeEnv)
			if err != nil {
				log.Println(err)
				return
			}

			log.Printf("[VirtController] Draining connection %s\n", inst.GetID())
			time.Sleep(time.Duration(drainingTime) * time.Second)
		}()

	}

	go func() {
		inst.DeRegisterPromDiscovery()
	}()

	if err := inst.Shutdown(); err != nil {
		log.Println(err)
		return err
	}

	m.Lock()
	delete(m.MapInstanceIdToInstance, inst.GetID())
	m.Unlock()

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
	log.Println("[VirtController] Closing virt connection")
	m.conn.Close()
	log.Println("[VirtController] Closed virt connection")

}

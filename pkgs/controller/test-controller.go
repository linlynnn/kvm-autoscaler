package controller

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/linlynnn/kvm-autoscaler/pkgs/instance"
)

type TestVmController struct {
	sync.Mutex
	MapInstanceIdToInstance map[string]instance.InstanceManager
	LastScaleUp             time.Time
	LastScaleDown           time.Time
	ScaleUpCoolDown         time.Duration
	ScaleDownCoolDown       time.Duration
}

func NewTestVmController(
	scaleUpCoolDown time.Duration,
	scaleDownCoolDown time.Duration,
) *TestVmController {

	now := time.Now()
	lastScaleUp := now.Add(-scaleUpCoolDown - (1 * time.Second))
	lastScaleDown := now.Add(-scaleDownCoolDown - (1 * time.Second))

	return &TestVmController{
		MapInstanceIdToInstance: make(map[string]instance.InstanceManager),
		LastScaleUp:             lastScaleUp,
		LastScaleDown:           lastScaleDown,
		ScaleUpCoolDown:         scaleUpCoolDown,
		ScaleDownCoolDown:       scaleDownCoolDown,
	}

}

func (m *TestVmController) ScaleUp(numToAdd int) {
	m.Lock()
	defer m.Unlock()

	now := time.Now()

	if now.Sub(m.LastScaleUp) < m.ScaleUpCoolDown {
		// ScaleUp is cooldown
		log.Println("ScaleUp is cooldown")
		return
	}

	log.Printf("Start ScaleUp %d\n", numToAdd)
	// ScaleUp logic
	for i := 0; i < numToAdd; i++ {
		// can be concurrent
		m.createVMTest()
	}

	m.LastScaleUp = now
	m.LastScaleDown = now

}

func (m *TestVmController) ScaleDown(instancesToRemove []instance.InstanceManager) {
	m.Lock()
	defer m.Unlock()
	now := time.Now()

	if now.Sub(m.LastScaleDown) < m.ScaleDownCoolDown {
		// ScaleDown is cooldown
		log.Println("ScaleDown is cooldown")
		return
	}

	// ScaleDown logic
	log.Println("Start ScaleDown")
	for _, instance := range instancesToRemove {
		// can be concurrent
		m.gracefullyShutdownTest(instance)

	}
	m.LastScaleDown = now

}

func (m *TestVmController) GetRunningInstance() (int, []instance.InstanceManager, error) {

	runningInstances := []instance.InstanceManager{}

	for _, instanceMng := range m.MapInstanceIdToInstance {
		instanceStatus := instanceMng.GetStatus()

		if instanceStatus == instance.VM_STATE_RUNNING {
			runningInstances = append(runningInstances, instanceMng)
		}

	}

	return len(runningInstances), runningInstances, nil

}

func (m *TestVmController) createVMTest() error {
	// pretend that creating
	log.Println("Creating VM ...")
	time.Sleep(5 * time.Second)
	bootTime := time.Now()
	uuid := uuid.New()
	instanceId := "instance-" + uuid.String()
	testInstanceMng := instance.NewTestInstanceManager(bootTime, instanceId)
	m.MapInstanceIdToInstance[uuid.String()] = testInstanceMng
	log.Printf("Created VM: %s\n", uuid)
	// maybe run the InstanceManager in background for doing something

	// register ip

	return nil

}

func (m *TestVmController) gracefullyShutdownTest(instance instance.InstanceManager) error {
	bootTime := instance.GetBootTime().String()
	instanceId := instance.GetID()
	instance.Shutdown()
	delete(m.MapInstanceIdToInstance, instanceId)
	log.Printf("Deleted Instance %s, bootTime: %s\n", instanceId, bootTime)
	return nil

}

func (m *TestVmController) Close() {

}

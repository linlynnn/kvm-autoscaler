package instance

import (
	"context"
	"log"
	"time"
)

type TestInstanceManager struct {
	bootTime time.Time
	state    VMState
	id       string
}

func NewTestInstanceManager(bootTime time.Time, id string) *TestInstanceManager {
	return &TestInstanceManager{
		bootTime: bootTime,
		state:    VM_STATE_RUNNING,
		id:       id,
	}

}

func (dm *TestInstanceManager) GetStatus() VMState {
	return dm.state

}

func (dm *TestInstanceManager) GetBootTime() time.Time {
	return dm.bootTime

}

func (dm *TestInstanceManager) GetID() string {
	return dm.id

}

func (dm *TestInstanceManager) RegisterIP(lbAddress string, ctx context.Context) {

}

func (dm *TestInstanceManager) DeRegisterIP(ipAddress string) {

}

func (dm *TestInstanceManager) Shutdown() error {
	dm.state = VM_STATE_SHUTTING_DOWN
	log.Printf("Shutting down VM: %s\n", dm.GetID())
	time.Sleep(2 * time.Second)
	dm.state = VM_STATE_SHUT_OFF
	log.Printf("Shut off VM: %s\n", dm.GetID())
	return nil

}

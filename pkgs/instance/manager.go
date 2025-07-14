package instance

import (
	"context"
	"time"
)

type InstanceManager interface {
	GetStatus() VMState
	GetBootTime() time.Time
	GetID() string
	Shutdown() error
	RegisterIP(string, context.Context)
	DeRegisterIP(string)
	RegisterPromDiscovery()
	DeRegisterPromDiscovery()
}

type VMState int

const (
	VM_STATE_RUNNING VMState = iota
	VM_STATE_STOPPING
	VM_STATE_SHUTTING_DOWN
	VM_STATE_SHUT_OFF
)

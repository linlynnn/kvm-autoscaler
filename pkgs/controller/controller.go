package controller

import "github.com/linlynnn/kvm-autoscaler/pkgs/instance"

type VmController interface {
	ScaleUp(numToAdd int)
	ScaleDown(instancesToRemove []instance.InstanceManager)
	GetRunningInstance() (int, []instance.InstanceManager, error)
	Close()
}

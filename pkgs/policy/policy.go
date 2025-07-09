package policy

import "github.com/linlynnn/kvm-autoscaler/pkgs/controller"

type ScalingPolicy interface {
	Apply()
	AttachVmController(controller.VmController)
}

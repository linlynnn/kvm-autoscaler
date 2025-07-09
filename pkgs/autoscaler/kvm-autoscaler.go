package autoscaler

import (
	"log"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"github.com/linlynnn/kvm-autoscaler/pkgs/controller"
	"github.com/linlynnn/kvm-autoscaler/pkgs/lb"
	"github.com/linlynnn/kvm-autoscaler/pkgs/policy"
	"libvirt.org/go/libvirt"
)

type KVMAutoScaler struct {
	scalingPolicies []policy.ScalingPolicy
	vmController    controller.VmController
	loadBalancer    *lb.LoadBalancer
}

func New(loadBalancer *lb.LoadBalancer) *KVMAutoScaler {

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatalf("Failed to connect to hypervisor: %v", err)
	}

	virtController := controller.NewVirtController(
		conn,
		30*time.Second,
		30*time.Second,
		loadBalancer,
	)

	return &KVMAutoScaler{
		scalingPolicies: []policy.ScalingPolicy{},
		vmController:    virtController,
		loadBalancer:    loadBalancer,
	}

}

func (a *KVMAutoScaler) AttachPolicy(policies []policy.ScalingPolicy) {
	for _, policy := range policies {
		policy.AttachVmController(a.vmController)
	}
	a.scalingPolicies = policies
}

func (a *KVMAutoScaler) Run() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	var wg sync.WaitGroup

	for _, policy := range a.scalingPolicies {
		wg.Add(1)
		go policy.Apply()

	}

	if a.loadBalancer != nil {
		wg.Add(1)
		go a.loadBalancer.Run()
	}

	wg.Wait()
	a.vmController.Close()

}

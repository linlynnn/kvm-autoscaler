package instance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	libvirt "libvirt.org/go/libvirt"
)

type VirtInstanceManager struct {
	// InstanceConn
	id        string
	domain    *libvirt.Domain
	bootTime  time.Time
	ipAddress string
}

func NewVirtInstanceManager(
	domain *libvirt.Domain,
	instanceId string,
) *VirtInstanceManager {
	bootTime := time.Now()

	return &VirtInstanceManager{
		domain:   domain,
		id:       instanceId,
		bootTime: bootTime,
	}

}

func (d *VirtInstanceManager) GetID() string {
	return d.id

}

func (d *VirtInstanceManager) RegisterIP(lbUrl string, ctx context.Context) {

	log.Printf("[RegisterIP] Registering IP for VM %s\n", d.GetID())
	//timeout := time.After(1 * time.Minute)
	tick := time.Tick(2 * time.Second)
	var ipAddress string

OuterLoop:
	for {
		select {
		case <-ctx.Done():
			log.Printf("[RegisterIP] Timeout: no IP found for VM %s\n", d.GetID())
			return

		case <-tick:
			ifaces, err := d.domain.ListAllInterfaceAddresses(libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE)
			if err != nil {
				log.Printf("[RegisterIP] Retrying... failed to get interface addresses: %v", err)
				continue
			}

			for _, iface := range ifaces {
				for _, addr := range iface.Addrs {
					if addr.Addr != "" {
						log.Printf("[RegisterIP] Found IP for VM %s: %s\n", d.GetID(), addr.Addr)
						ipAddress = addr.Addr
						// TODO: register/store this IP in your system

						break OuterLoop
					}
				}
			}
		}
	}

	if ipAddress == "" {
		return
	}

	d.ipAddress = ipAddress

	coldStartTimeoutEnv := os.Getenv("COLD_START_TIMEOUT_MIN")
	if coldStartTimeoutEnv == "" {
		log.Println("[RegisterIP] COLD_START_TIMEOUT_MIN is not defined, use fallback value 8")
		coldStartTimeoutEnv = "8"
	}

	coldStartTimeout, err := strconv.Atoi(coldStartTimeoutEnv)
	if err != nil {
		log.Println(err)
	}

	log.Printf("[RegisterIP] Wait for vm %s startup application for %d minute\n", ipAddress, coldStartTimeout)
	time.Sleep(time.Duration(coldStartTimeout) * time.Minute)

	lbUrl = lbUrl + "/backend"

	payload := map[string]string{
		"name": d.GetID(),
		"url":  "http://" + ipAddress + ":" + os.Getenv("TARGET_PORT"),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Println(err)
		return
	}

	resp, err := http.Post(lbUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("[RegisterIP] Error reading response:", err)
		return
	}

	log.Printf("[RegisterIP] RegisterIP response %s\n", string(body))

	log.Printf("[RegisterIP] Done RegisteringIP %s\n", d.GetID())

}

func (d *VirtInstanceManager) GetBootTime() time.Time {
	return d.bootTime
}

func (d *VirtInstanceManager) GetStatus() VMState {
	// Get the VM state
	state, _, err := d.domain.GetState()
	if err != nil {
		log.Printf("Failed to get domain state: %v\n", err)
		return VM_STATE_SHUT_OFF

	}

	switch state {
	case libvirt.DOMAIN_RUNNING:
		return VM_STATE_RUNNING
	case libvirt.DOMAIN_SHUTDOWN:
		return VM_STATE_SHUTTING_DOWN
	case libvirt.DOMAIN_SHUTOFF:
		return VM_STATE_SHUT_OFF
	}

	return VM_STATE_RUNNING

}

func (d *VirtInstanceManager) DeRegisterIP(lbUrl string) {
	lbUrl = lbUrl + "/backend"

	payload := map[string]string{
		"url": "http://" + d.ipAddress + ":" + os.Getenv("TARGET_PORT"),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Println("[DeRegisterIP] Error marshaling payload:", err)
		return
	}

	req, err := http.NewRequest(http.MethodDelete, lbUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("[DeRegisterIP] Error creating DELETE request:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("[DeRegisterIP] Error performing DELETE request:", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("[DeRegisterIP] DeRegisterIP response: %s\n", resp.Status)
}

func (d *VirtInstanceManager) Shutdown() error {
	// virt shutdown implementation

	log.Printf("[Shutdown] Shutting Down VM %s\n", d.GetID())
	if err := d.domain.Destroy(); err != nil {
		log.Println(err)
		return err
	}
	log.Printf("[Shutdown] Shut off VM %s\n", d.GetID())

	log.Printf("[Shutdown] Undefining VM %s\n", d.GetID())
	if err := d.domain.Undefine(); err != nil {
		log.Println(err)
		return err
	}
	log.Printf("[Shutdown] Undefined VM %s\n", d.GetID())

	return nil

}

func (d *VirtInstanceManager) RegisterPromDiscovery() {

	coldStartTimeoutEnv := os.Getenv("COLD_START_TIMEOUT_MIN")
	if coldStartTimeoutEnv == "" {
		log.Println("[RegisterPrometheusDiscovery] COLD_START_TIMEOUT_MIN is not defined, use fallback value 8")
		coldStartTimeoutEnv = "8"
	}

	coldStartTimeout, err := strconv.Atoi(coldStartTimeoutEnv)
	if err != nil {
		log.Println(err)
	}

	log.Printf("[RegisterPrometheusDiscovery] Wait for vm %s startup application for %d minute\n", d.ipAddress, coldStartTimeout)
	time.Sleep(time.Duration(coldStartTimeout) * time.Minute)

	discoveryUrl := "http://localhost:9093/targets/node_exporter"

	payload := map[string]string{
		"url": d.ipAddress + ":9100",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Println("[RegisterPrometheusDiscovery] Error marshaling payload:", err)
		return
	}

	resp, err := http.Post(discoveryUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()

	log.Printf("[RegisterPrometheusDiscovery] Register Prometheus Discovery response %s\n", string(resp.Status))

	log.Printf("[RegisterPrometheusDiscovery] Done Registering Prometheus Discovery %s\n", d.GetID())

}

func (d *VirtInstanceManager) DeRegisterPromDiscovery() {
	discoveryUrl := "http://localhost:9093/targets/node_exporter"

	payload := map[string]string{
		"url": d.ipAddress + ":9100",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Println("[DeRegisterPromDiscovery] Error marshaling payload:", err)
		return
	}

	req, err := http.NewRequest(http.MethodDelete, discoveryUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("[DeRegisterPromDiscovery] Error creating DELETE request:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("[DeRegisterPromDiscovery] Error performing DELETE request:", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("[DeRegisterPromDiscovery] DeRegister Prometheus Discovery response: %s\n", resp.Status)

}

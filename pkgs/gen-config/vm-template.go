package genconfig

func GetVirtTemplate() string {
	return `<domain type='kvm'>
  <name>{{.DOMAIN_NAME}}</name>
  <memory unit='MiB'>{{.INSTANCE_MEMORY}}</memory>
  <vcpu placement='static'>{{.INSTANCE_VCPU}}</vcpu>

  <os>
    <type arch='x86_64' machine='pc'>hvm</type>
  </os>

  <features>
    <acpi/>
    <apic/>
    <pae/>
  </features>

  <cpu mode='host-model'/>
  <clock offset='utc'/>
  <on_poweroff>destroy</on_poweroff>
  <on_reboot>restart</on_reboot>
  <on_crash>restart</on_crash>

  <devices>
    <channel type='unix'>
      <source mode='bind' path='/var/lib/libvirt/qemu/{{.GA_SOCKET_NAME}}.agent'/>
      <target type='virtio' name='org.qemu.guest_agent.0'/>
    </channel>
    
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/var/lib/libvirt/images/{{.OVERLAY_IMAGE}}.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>

    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='/var/lib/libvirt/images/{{.CDROM_IMAGE}}.iso'/>
      <target dev='hdb' bus='ide'/>
      <readonly/>
    </disk>

    <interface type='network'>
      <source network='default'/>
      <model type='virtio'/>
    </interface>

    <console type='pty'>
      <target type='serial' port='0'/>
    </console>

    <serial type='pty'>
      <target port='0'/>
    </serial>
  </devices>
</domain>

	`

}

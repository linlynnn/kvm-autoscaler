package genconfig

import (
	"log"
	"os"
	"os/exec"
	"path"
	"text/template"
)

// overlay: {jammy-base-image}
func GenQcow2DiskImage(id string) error {
	//sudo qemu-img create -f qcow2 -b "$BASE_IMAGE" -F qcow2 "${IMG_REGISTRY}/${OVERLAY_IMAGE}" 5G
	log.Println("Generating qcow2 disk-image:", id)

	cmd := exec.Command("sudo", "qemu-img", "create", "-f", "qcow2", "-b",
		"jammy-server-cloudimg-amd64.img",
		"-F",
		"qcow2",
		"/var/lib/libvirt/images/overlay-"+id+".qcow2",
		"5G",
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err

	}
	log.Println("Generated cloud-init cdrom:", id)
	return nil

}

// config: {meta-data, user-data}
func GenCdRomDiskImage(id string) error {
	//sudo cloud-localds "${IMG_REGISTRY}/${CDROM_IMAGE}" "${USER_DIR}/${USER_DEST}" "${META_DIR}/${META_DEST}"
	log.Println("Generating cloud-init cdrom:", id)

	cmd := exec.Command("sudo", "cloud-localds",
		"/var/lib/libvirt/images/cdrom-"+id+".iso",
		"output/user-data/user-data-"+id,
		"output/meta-data/meta-data-"+id,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err

	}
	log.Println("Generated cloud-init cdrom:", id)
	return nil

}

func GenMetaDataInstanceConfig(
	id string,
) error {

	log.Println("Generating meta-data: ", id)
	err := os.MkdirAll("output/meta-data", 0755)
	if err != nil {
		log.Println(err)
	}

	// In GenMetaDataInstanceConfig, load embedded templates instead of reading from disk
	tpl, err := LoadTemplates()
	if err != nil {
		return err
	}
	metaData := map[string]string{
		"INSTANCE_ID":    "instance-" + id,
		"LOCAL_HOSTNAME": "instance-" + id,
	}

	outputFileName := "meta-data-" + id
	outputFilePath := path.Join("output/meta-data", outputFileName)
	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return nil
	}
	defer outFile.Close()

	err = tpl.ExecuteTemplate(outFile, "meta-data.tmpl", metaData)
	if err != nil {
		return nil
	}

	log.Println("Generated meta-data: ", id)

	return nil
}

func GenUserDataInstanceConfig(
	id string,
	sshPublicKey string,
) error {

	log.Println("Generating user-data: ", id)
	err := os.MkdirAll("output/user-data", 0755)
	if err != nil {
		log.Println(err)
	}

	// In GenUserDataInstanceConfig, load embedded templates instead of reading from disk
	tpl, err := LoadTemplates()
	if err != nil {
		return err
	}
	metaData := map[string]string{
		"HOSTNAME":       "instance-" + id,
		"SSH_PUBLIC_KEY": sshPublicKey,
	}

	outputFileName := "user-data-" + id
	outputFilePath := path.Join("output/user-data", outputFileName)
	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return nil
	}
	defer outFile.Close()

	err = tpl.ExecuteTemplate(outFile, "user-data.tmpl", metaData)
	if err != nil {
		return nil
	}

	log.Println("Generated user-data: ", id)

	return nil
}

func GenVirtInstanceConfig(
	id string,

) (string, error) {

	log.Println("Generating virt config: ", id)
	err := os.MkdirAll("output/virt-config", 0755)
	if err != nil {
		return "", err
	}

	virtTemplate := GetVirtTemplate()
	virtTmpl, err := template.New("virt-config").Parse(virtTemplate)
	if err != nil {
		return "", err
	}

	virtData := map[string]string{
		"DOMAIN_NAME":    "instance-" + id,
		"GA_SOCKET_NAME": "ga-socket-" + id,
		"OVERLAY_IMAGE":  "overlay-" + id,
		"CDROM_IMAGE":    "cdrom-" + id,
	}

	outputFileName := "instance-" + id
	outputFilePath := path.Join("output/virt-config", outputFileName)
	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	err = virtTmpl.Execute(outFile, virtData)
	if err != nil {
		return "", nil
	}
	log.Println("Generated xml config: ", id)

	return outputFilePath, nil
}

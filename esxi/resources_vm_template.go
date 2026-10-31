package esxi

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourceVMTEMPLATE() *schema.Resource {
	return &schema.Resource{
		Create: resourceVMTEMPLATECreate,
		Read:   resourceVMTEMPLATERead,
		Update: nil,
		Delete: resourceVMTEMPLATEDelete,

		Schema: map[string]*schema.Schema{
			"source_path": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Local path to the source VM folder on your machine.",
			},
			"source_name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Source VM name (vmdk filename without extension).",
			},
			"disk_store": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Destination datastore on ESXi.",
			},
			"vmdk_path": &schema.Schema{
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Full path to the VMDK on ESXi, to be used by esxi_guest_clone.",
			},
			"dst_path": &schema.Schema{
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Full path to the template folder on ESXi.",
			},
		},
	}
}

func resourceVMTEMPLATECreate(d *schema.ResourceData, m interface{}) error {
	c := m.(*Config)
	log.Printf("[resourceVMTEMPLATECreate]\n")

	source_path := d.Get("source_path").(string)
	source_name := d.Get("source_name").(string)
	disk_store := d.Get("disk_store").(string)

	if source_path == "" {
		return errors.New("source_path cannot be empty")
	}
	if err := validateName("source_name", source_name); err != nil {
		return err
	}
	if err := validateName("disk_store", disk_store); err != nil {
		return err
	}

	dst_path := fmt.Sprintf("/vmfs/volumes/%s/templates/%s", disk_store, source_name)
	vmdk_path := fmt.Sprintf("%s/%s.vmdk", dst_path, source_name)

	esxiConnInfo := getConnectionInfo(c)

	// Create template folder on ESXi
	stdout, err := runRemoteSshCommand(esxiConnInfo,
		fmt.Sprintf("mkdir -p \"%s\"", dst_path),
		"create template folder")
	if err != nil {
		return fmt.Errorf("Failed to create template folder: %s\n%s", err, strings.TrimSpace(stdout))
	}

	// Disk, flat disk, then config files
	if err := copyFileToEsxi(c, filepath.Join(source_path, source_name+".vmdk"), fmt.Sprintf("%s/%s.vmdk", dst_path, source_name)); err != nil {
		return fmt.Errorf("Failed to copy VMDK: %s", err)
	}
	if err := copyFileToEsxi(c, filepath.Join(source_path, source_name+"-flat.vmdk"), fmt.Sprintf("%s/%s-flat.vmdk", dst_path, source_name)); err != nil {
		return fmt.Errorf("Failed to copy flat VMDK: %s", err)
	}
	if err := copyVMConfigFiles(c, source_path, source_name, dst_path); err != nil {
		return fmt.Errorf("Failed to copy config files: %s", err)
	}

	// Store computed values
	d.Set("vmdk_path", vmdk_path)
	d.Set("dst_path", dst_path)
	d.SetId(dst_path)

	return resourceVMTEMPLATERead(d, m)
}

func resourceVMTEMPLATERead(d *schema.ResourceData, m interface{}) error {
	c := m.(*Config)
	esxiConnInfo := getConnectionInfo(c)

	dst_path := strings.TrimSpace(d.Get("dst_path").(string))
	if dst_path == "" {
		return errors.New("dst_path is empty, cannot read template state")
	}

	// Check if template folder still exists
	_, err := runRemoteSshCommand(esxiConnInfo,
		fmt.Sprintf("ls \"%s\"", dst_path),
		"check template folder")
	if err != nil {
		log.Printf("[resourceVMTEMPLATERead] Template folder not found, marking as deleted\n")
		d.SetId("")
	}

	return nil
}

func resourceVMTEMPLATEDelete(d *schema.ResourceData, m interface{}) error {
	c := m.(*Config)
	esxiConnInfo := getConnectionInfo(c)
	log.Printf("[resourceVMTEMPLATEDelete]\n")

	dst_path := strings.TrimSpace(d.Get("dst_path").(string))
	if dst_path == "" {
		return errors.New("dst_path is empty, cannot delete template")
	}
	if err := validateTemplatePath(dst_path); err != nil {
		return fmt.Errorf("refusing to delete: %s", err)
	}

	stdout, err := runRemoteSshCommand(esxiConnInfo,
		fmt.Sprintf("rm -rf \"%s\"", dst_path),
		"delete template folder")
	if err != nil {
		return fmt.Errorf("Failed to delete template: %s\n%s", err, strings.TrimSpace(stdout))
	}

	log.Printf("[resourceVMTEMPLATEDelete] Template deleted: %s\n", dst_path)
	return nil
}

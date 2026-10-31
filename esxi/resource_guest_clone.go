package esxi

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
)

func resourceGUESTCLONE() *schema.Resource {
	return &schema.Resource{
		Create: resourceGUESTCLONECreate,
		Read:   resourceGUESTRead,
		Update: resourceGUESTCLONEUpdate,
		Delete: resourceGUESTDelete,
		Importer: &schema.ResourceImporter{
			State: resourceGUESTImport,
		},
		Schema: map[string]*schema.Schema{
			"template_vmdk_path": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Path to a template VMDK already on the ESXi host (see esxi_vm_template). If set, the disk is cloned on the host with vmkfstools instead of being copied from source_path.",
			},
			"source_path": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Local folder holding the source VM files (.vmx, .nvram, .vmdk ...).",
			},
			"source_name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Source VM name (vmdk and vmx filename without extension).",
			},
			"guest_name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the cloned VM.",
			},
			"disk_store": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Destination datastore.",
			},
			"resource_pool_name": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "/",
				Description: "Resource pool name to place guest.",
			},
			"network_interfaces": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: false,
				Default:  nil,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"virtual_network": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
							Computed: true,
						},
						"mac_address": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
							Computed: true,
						},
						"nic_type": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
							Computed: true,
						},
					},
				},
			},
			"power": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    false,
				Computed:    true,
				Description: "Guest power state.",
			},
			"ip_address": &schema.Schema{
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The IP address reported by VMware tools.",
			},
			"guest_startup_timeout": {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				Description:  "Seconds to wait for an available IP address.",
				ValidateFunc: validation.IntBetween(0, 600),
			},
			"guest_shutdown_timeout": {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				Description:  "Seconds to wait for graceful shutdown.",
				ValidateFunc: validation.IntBetween(0, 600),
			},
			"notes": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    false,
				Computed:    true,
				Description: "Guest notes (annotation).",
			},
		},
	}
}

func resourceGUESTCLONECreate(d *schema.ResourceData, m interface{}) error {
	c := m.(*Config)
	log.Printf("[resourceGUESTCLONECreate]\n")

	source_path := d.Get("source_path").(string)
	source_name := d.Get("source_name").(string)
	guest_name := d.Get("guest_name").(string)
	disk_store := d.Get("disk_store").(string)
	resource_pool_name := d.Get("resource_pool_name").(string)
	power := d.Get("power").(string)
	template_vmdk_path := d.Get("template_vmdk_path").(string)

	// The config files (.vmx...) always come from source_path
	if source_path == "" {
		return errors.New("source_path must be provided")
	}

	// Validate names and paths: they are used inside shell commands on the host
	if err := validateName("source_name", source_name); err != nil {
		return err
	}
	if err := validateName("guest_name", guest_name); err != nil {
		return err
	}
	if err := validateName("disk_store", disk_store); err != nil {
		return err
	}
	if template_vmdk_path != "" {
		if err := validateRemotePath("template_vmdk_path", template_vmdk_path); err != nil {
			return err
		}
	}
	if resource_pool_name != "" {
		if err := validatePoolName(resource_pool_name); err != nil {
			return err
		}
	}

	// Default timeouts
	if d.Get("guest_startup_timeout").(int) == 0 {
		d.Set("guest_startup_timeout", 120)
	}
	if d.Get("guest_shutdown_timeout").(int) == 0 {
		d.Set("guest_shutdown_timeout", 20)
	}

	// Validate resource pool
	if resource_pool_name == "ha-root-pool" {
		resource_pool_name = "/"
	}

	// Clone the VM
	vmid, err := guestCLONE(c, source_path, source_name, guest_name, disk_store, resource_pool_name, template_vmdk_path)
	if err != nil {
		return fmt.Errorf("Failed to clone VM: %s", err)
	}

	d.SetId(vmid)

	// Power on if requested
	if power == "on" || power == "" {
		_, err = guestPowerOn(c, vmid)
		if err != nil {
			return errors.New("Failed to power on.")
		}

		// Answer ESXi "moved or copied" question
		err = guestAnswerCopied(c, vmid)
		if err != nil {
			log.Printf("[resourceGUESTCLONECreate] Warning: %s\n", err)
		}
	}

	d.Set("power", "on")
	return resourceGUESTRead(d, m)
}

func resourceGUESTCLONEUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceGUESTRead(d, m)
}

func guestCLONE(c *Config, source_path string, source_name string, guest_name string, disk_store string, resource_pool_name string, template_vmdk_path string) (string, error) {
	esxiConnInfo := getConnectionInfo(c)
	log.Printf("[guestCLONE]\n")

	dst_path := fmt.Sprintf("/vmfs/volumes/%s/%s", disk_store, guest_name)

	// Create destination folder on ESXi
	stdout, err := runRemoteSshCommand(esxiConnInfo,
		fmt.Sprintf("mkdir \"%s\"", dst_path),
		"create destination folder")
	if err != nil {
		return "", fmt.Errorf("Failed to create destination folder: %s\n%s", err, strings.TrimSpace(stdout))
	}

	if template_vmdk_path != "" {
		// Clone VMDK from template using vmkfstools
		dst_vmdk := fmt.Sprintf("%s/%s.vmdk", dst_path, guest_name)
		cmd := fmt.Sprintf("vmkfstools -i \"%s\" \"%s\" -d thin", template_vmdk_path, dst_vmdk)
		stdout, err := runRemoteSshCommand(esxiConnInfo, cmd, "clone vmdk from template")
		if err != nil {
			return "", fmt.Errorf("Failed to clone VMDK with vmkfstools: %s\n%s", err, strings.TrimSpace(stdout))
		}
		log.Printf("[guestCLONE] vmkfstools output: %s\n", strings.TrimSpace(stdout))

		// Config files only
		if err := copyVMConfigFiles(c, source_path, source_name, dst_path); err != nil {
			return "", fmt.Errorf("Failed to copy config files: %s", err)
		}
	} else {
		// Config files
		if err := copyVMConfigFiles(c, source_path, source_name, dst_path); err != nil {
			return "", fmt.Errorf("Failed to copy config files: %s", err)
		}

		// VMDK and flat VMDK with the new name
		if err := copyFileToEsxi(c, filepath.Join(source_path, source_name+".vmdk"), fmt.Sprintf("%s/%s.vmdk", dst_path, guest_name)); err != nil {
			return "", fmt.Errorf("Failed to copy VMDK: %s", err)
		}
		if err := copyFileToEsxi(c, filepath.Join(source_path, source_name+"-flat.vmdk"), fmt.Sprintf("%s/%s-flat.vmdk", dst_path, guest_name)); err != nil {
			return "", fmt.Errorf("Failed to copy flat VMDK: %s", err)
		}
	}

	// Update display name in VMX
	dst_vmx := fmt.Sprintf("%s/%s.vmx", dst_path, source_name)
	cmd2 := fmt.Sprintf("sed -i 's/displayName = \"%s\"/displayName = \"%s\"/' \"%s\"", sedEscape(source_name), guest_name, dst_vmx)
	stdout, err = runRemoteSshCommand(esxiConnInfo, cmd2, "update display name")
	if err != nil {
		return "", fmt.Errorf("Failed to update VMX display name: %s\n%s", err, strings.TrimSpace(stdout))
	}

	// Update VMDK filename in VMX
	cmd3 := fmt.Sprintf("sed -i 's/scsi0:0.fileName = \"%s.vmdk\"/scsi0:0.fileName = \"%s.vmdk\"/' \"%s\"", sedEscape(source_name), guest_name, dst_vmx)
	stdout, err = runRemoteSshCommand(esxiConnInfo, cmd3, "update vmdk filename")
	if err != nil {
		return "", fmt.Errorf("Failed to update VMDK filename in VMX: %s\n%s", err, strings.TrimSpace(stdout))
	}

	// Register VM
	poolID, err := getPoolID(c, resource_pool_name)
	if err != nil {
		return "", fmt.Errorf("Failed to get resource pool ID: %s", err)
	}

	stdout, err = runRemoteSshCommand(esxiConnInfo,
		fmt.Sprintf("vim-cmd solo/registervm \"%s\" %s %s", dst_vmx, guest_name, poolID),
		"register VM")
	if err != nil {
		return "", fmt.Errorf("Failed to register VM: %s\n%s", err, strings.TrimSpace(stdout))
	}

	vmid := strings.TrimSpace(stdout)
	log.Printf("[guestCLONE] VM registered with ID: %s\n", vmid)

	return vmid, nil
}

func guestAnswerCopied(c *Config, vmid string) error {
	esxiConnInfo := getConnectionInfo(c)
	log.Printf("[guestAnswerCopied]\n")

	// Wait for ESXi "moved or copied" question (max 30s)
	for i := 0; i < 30; i++ {
		stdout, _ := runRemoteSshCommand(esxiConnInfo,
			fmt.Sprintf("vim-cmd vmsvc/message %s", vmid),
			"get VM message")

		if strings.Contains(stdout, "Virtual machine message") {
			lines := strings.Split(stdout, "\n")
			for _, line := range lines {
				if strings.Contains(line, "Virtual machine message") {
					parts := strings.Fields(line)
					if len(parts) >= 4 {
						questionID := strings.TrimSuffix(parts[3], ":")
						_, err := runRemoteSshCommand(esxiConnInfo,
							fmt.Sprintf("vim-cmd vmsvc/message %s %s 2", vmid, questionID),
							"answer copied question")
						if err != nil {
							return fmt.Errorf("Failed to answer question: %s", err)
						}
						log.Printf("[guestAnswerCopied] Answered 'I Copied It' for VM %s\n", vmid)
						return nil
					}
				}
			}
		}

		runRemoteSshCommand(esxiConnInfo, "sleep 1", "wait for question")
	}

	return fmt.Errorf("Timeout waiting for copied/moved question")
}

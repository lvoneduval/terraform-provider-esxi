package esxi

import (
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourceVIRTUALDISKRead(d *schema.ResourceData, m interface{}) error {
	c := m.(*Config)
	log.Println("[resourceVIRTUALDISKRead]")

	virtual_disk_disk_store, virtual_disk_dir, virtual_disk_name, virtual_disk_size, virtual_disk_type, err := virtualDiskREAD(c, d.Id())
	if errors.Is(err, errVirtualDiskNotFound) {
		// The disk was removed outside of terraform: forget it so it can be recreated.
		log.Printf("[resourceVIRTUALDISKRead] %s\n", err)
		d.SetId("")
		return nil
	}
	if err != nil {
		d.SetId("")
		return fmt.Errorf("Failed to refresh virtual disk: %s\n", err)
	}

	d.Set("virtual_disk_disk_store", virtual_disk_disk_store)
	d.Set("virtual_disk_dir", virtual_disk_dir)
	d.Set("virtual_disk_name", virtual_disk_name)
	d.Set("virtual_disk_size", virtual_disk_size)
	if virtual_disk_type != "Unknown" {
		d.Set("virtual_disk_type", virtual_disk_type)
	}

	return nil
}

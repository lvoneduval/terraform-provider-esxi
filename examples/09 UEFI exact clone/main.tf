#########################################
#  ESXI Provider host/login details
#########################################
provider "esxi" {
  esxi_hostname = var.esxi_hostname
  esxi_hostport = var.esxi_hostport
  esxi_hostssl  = var.esxi_hostssl
  esxi_username = var.esxi_username
  esxi_password = var.esxi_password
}

#########################################
#  Template
#########################################
#
#  Copies a powered off UEFI VM from a local folder to the esxi host, once.
#  The folder must contain debian-uefi.vmx, debian-uefi.nvram, debian-uefi.vmdk
#  and debian-uefi-flat.vmdk.  The files are copied exactly as they are, which keeps
#  the UEFI VM bootable.
#
resource "esxi_vm_template" "debian_uefi" {
  source_path = "/my_local_system_path/debian-uefi"
  source_name = "debian-uefi"
  disk_store  = "DS_001"
}

#########################################
#  Guests cloned from the template
#########################################
#
#  The disk is cloned on the esxi host with vmkfstools.  The .vmx and .nvram come from
#  source_path.  Without template_vmdk_path, the disk is copied from source_path instead.
#
resource "esxi_guest_clone" "vmtest09" {
  guest_name         = "vmtest09"
  source_path        = "/my_local_system_path/debian-uefi"
  source_name        = "debian-uefi"
  disk_store         = "DS_001"
  template_vmdk_path = esxi_vm_template.debian_uefi.vmdk_path
  resource_pool_name = "/"
  power              = "on"
}

output "vmtest09_ip" {
  value = esxi_guest_clone.vmtest09.ip_address
}

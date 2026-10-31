package esxi

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tmc/scp"
)

// Names and paths end up inside shell commands run on the ESXi host
// (mkdir, sed, rm -rf, vmkfstools...), so only a plain set of characters is
// accepted: no quotes, no $, no backticks, no ';', no '..'.
var (
	safeNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._ -]*$`)
	safePathRe = regexp.MustCompile(`^/[A-Za-z0-9._/ -]*$`)
	safePoolRe = regexp.MustCompile(`^[A-Za-z0-9/][A-Za-z0-9._/ -]*$`)
)

func validateName(label, value string) error {
	if !safeNameRe.MatchString(value) || strings.Contains(value, "..") {
		return fmt.Errorf("%s %q is not valid: use only letters, digits, space, '.', '_' and '-', starting with a letter or digit, without '..'", label, value)
	}
	return nil
}

func validateRemotePath(label, value string) error {
	if !safePathRe.MatchString(value) || strings.Contains(value, "..") {
		return fmt.Errorf("%s %q is not valid: use an absolute path with only letters, digits, space, '.', '_', '-' and '/', without '..'", label, value)
	}
	return nil
}

func validatePoolName(value string) error {
	if !safePoolRe.MatchString(value) || strings.Contains(value, "..") {
		return fmt.Errorf("resource_pool_name %q is not valid", value)
	}
	return nil
}

// A template folder is only ever created under <datastore>/templates/, so the
// recursive delete refuses anything else (state file edited by hand, empty id...).
func validateTemplatePath(value string) error {
	if err := validateRemotePath("dst_path", value); err != nil {
		return err
	}
	if !strings.HasPrefix(value, "/vmfs/volumes/") || !strings.Contains(value, "/templates/") || strings.HasSuffix(value, "/templates/") || strings.HasSuffix(value, "/templates") {
		return fmt.Errorf("dst_path %q is not a template folder under /vmfs/volumes/<datastore>/templates/", value)
	}
	return nil
}

// copyFileToEsxi sends one local file to the ESXi host over the connection the
// provider already uses (same user, port and password as the other commands),
// then checks the size of the remote file.
func copyFileToEsxi(c *Config, localPath string, remotePath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %s", localPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", localPath)
	}

	esxiConnInfo := getConnectionInfo(c)
	client, session, err := connectToHost(esxiConnInfo, 3)
	if err != nil {
		return fmt.Errorf("cannot connect to the ESXi host: %s", err)
	}
	err = scp.CopyPath(localPath, remotePath, session)
	client.Close()
	if err != nil {
		return fmt.Errorf("copy of %s failed: %s", filepath.Base(localPath), err)
	}

	stdout, err := runRemoteSshCommand(esxiConnInfo, fmt.Sprintf("ls -l \"%s\"", remotePath), "check copied file")
	if err != nil {
		return fmt.Errorf("copy of %s could not be checked: %s\n%s", filepath.Base(localPath), err, stdout)
	}
	fields := strings.Fields(stdout)
	if len(fields) < 5 {
		return fmt.Errorf("copy of %s could not be checked: unexpected output %q", filepath.Base(localPath), stdout)
	}
	size, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil || size != info.Size() {
		return fmt.Errorf("copy of %s is incomplete: %s bytes on the host, %d expected", filepath.Base(localPath), fields[4], info.Size())
	}
	return nil
}

// Files copied next to the disk. The .vmx is required, the others only if present.
func copyVMConfigFiles(c *Config, source_path string, source_name string, dst_path string) error {
	required := fmt.Sprintf("%s.vmx", source_name)
	optional := []string{
		fmt.Sprintf("%s.nvram", source_name),
		fmt.Sprintf("%s.vmsd", source_name),
		fmt.Sprintf("%s.scoreboard", source_name),
		"vmware.log",
	}

	if err := copyFileToEsxi(c, filepath.Join(source_path, required), dst_path+"/"+required); err != nil {
		return err
	}
	for _, name := range optional {
		local := filepath.Join(source_path, name)
		if _, err := os.Stat(local); err != nil {
			continue
		}
		if err := copyFileToEsxi(c, local, dst_path+"/"+name); err != nil {
			return err
		}
	}
	return nil
}

// The dot is the only sed metacharacter a validated name can contain.
func sedEscape(name string) string {
	return strings.ReplaceAll(name, ".", "\\.")
}

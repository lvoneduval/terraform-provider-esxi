package esxi

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Replace a leading ~ with the home directory of the current user.
func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand %q: %s", path, err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}

// The ssh authentication methods to try, in order: the private key if one is
// configured, then the password.
func sshAuthMethods(esxiConnInfo ConnectionStruct) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if esxiConnInfo.privateKeyPath != "" {
		path, err := expandHomePath(esxiConnInfo.privateKeyPath)
		if err != nil {
			return nil, err
		}
		key, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("unable to read esxi_private_key: %s", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			var passphraseErr *ssh.PassphraseMissingError
			if errors.As(err, &passphraseErr) {
				return nil, fmt.Errorf("esxi_private_key %s is protected by a passphrase, which is not supported", path)
			}
			return nil, fmt.Errorf("unable to parse esxi_private_key %s: %s", path, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	methods = append(methods, ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		// Reply password to all questions
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = esxiConnInfo.pass
		}

		return answers, nil
	}))

	return methods, nil
}

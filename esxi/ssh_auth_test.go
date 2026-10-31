package esxi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestKey(t *testing.T, dir string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "id_test")
	data := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := ioutil.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSshAuthMethods(t *testing.T) {
	dir, err := ioutil.TempDir("", "sshauth")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	keyPath := writeTestKey(t, dir)

	// No key: the password only, as before.
	methods, err := sshAuthMethods(ConnectionStruct{pass: "secret"})
	if err != nil || len(methods) != 1 {
		t.Fatalf("without key: got %d methods, err %v", len(methods), err)
	}

	// With a key: key first, then the password.
	methods, err = sshAuthMethods(ConnectionStruct{pass: "secret", privateKeyPath: keyPath})
	if err != nil || len(methods) != 2 {
		t.Fatalf("with key: got %d methods, err %v", len(methods), err)
	}

	// A missing file is an error, not a silent fallback to the password.
	if _, err := sshAuthMethods(ConnectionStruct{privateKeyPath: filepath.Join(dir, "missing")}); err == nil {
		t.Error("a missing key file must be an error")
	}

	// A file that is not a key is an error.
	bad := filepath.Join(dir, "bad")
	if err := ioutil.WriteFile(bad, []byte("not a key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := sshAuthMethods(ConnectionStruct{privateKeyPath: bad}); err == nil {
		t.Error("an invalid key file must be an error")
	}
}

func TestExpandHomePath(t *testing.T) {
	dir, err := ioutil.TempDir("", "home")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	old, had := os.LookupEnv("HOME")
	os.Setenv("HOME", dir)
	defer func() {
		if had {
			os.Setenv("HOME", old)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	got, err := expandHomePath("~/.ssh/id_ed25519")
	if err != nil || got != filepath.Join(dir, ".ssh", "id_ed25519") {
		t.Errorf("got %q, err %v", got, err)
	}
	if got, _ := expandHomePath("/abs/key"); got != "/abs/key" {
		t.Errorf("absolute path changed: %q", got)
	}
	if got, _ := expandHomePath("~other/key"); !strings.HasPrefix(got, "~other") {
		t.Errorf("~user must be left alone: %q", got)
	}
}

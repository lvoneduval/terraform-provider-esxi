package esxi

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/crypto/ssh"
)

// A lost connection or any other failure must never be taken for a missing disk.
func TestVirtualDiskMissingIgnoresOtherErrors(t *testing.T) {
	others := []error{
		nil,
		errors.New("Client Connection Error"),
		fmt.Errorf("wrapped: %w", errors.New("dial tcp: i/o timeout")),
		&ssh.ExitError{}, // exit status 0, not 1
	}
	for _, err := range others {
		if virtualDiskMissing(err) {
			t.Errorf("%v must not be reported as a missing disk", err)
		}
	}
}

func TestErrVirtualDiskNotFoundIsMatchable(t *testing.T) {
	err := fmt.Errorf("%w: %s", errVirtualDiskNotFound, "/vmfs/volumes/ds/x/x.vmdk")
	if !errors.Is(err, errVirtualDiskNotFound) {
		t.Error("errors.Is must match the wrapped error")
	}
}

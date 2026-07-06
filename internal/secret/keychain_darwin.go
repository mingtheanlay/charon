//go:build darwin

package secret

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// ErrKeychainMissing is returned when the requested item does not exist.
var ErrKeychainMissing = errors.New("keychain item not found")

// KeychainRead returns the value under service, or ErrKeychainMissing if absent.
func KeychainRead(service string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", service, "-w")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if strings.Contains(errb.String(), "could not be found") {
			return "", ErrKeychainMissing
		}
		return "", err
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// KeychainWrite stores value under service, replacing any existing item.
func KeychainWrite(service, account, value string) error {
	// -U updates the item if it already exists.
	cmd := exec.Command("security", "add-generic-password",
		"-s", service, "-a", account, "-w", value, "-U")
	return cmd.Run()
}

// KeychainDelete removes the item; a missing item is not an error.
func KeychainDelete(service string) error {
	cmd := exec.Command("security", "delete-generic-password", "-s", service)
	if err := cmd.Run(); err != nil {
		// Deleting a non-existent item exits non-zero; treat as success.
		return nil
	}
	return nil
}

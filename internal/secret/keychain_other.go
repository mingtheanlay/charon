//go:build !darwin

package secret

import "errors"

// ErrKeychainMissing is returned when the requested item does not exist.
var ErrKeychainMissing = errors.New("keychain item not found")

// KeychainRead is unsupported off macOS; callers treat this as "absent".
func KeychainRead(service string) (string, error) { return "", ErrKeychainMissing }

// KeychainWrite is a no-op off macOS.
func KeychainWrite(service, account, value string) error { return nil }

// KeychainDelete is a no-op off macOS.
func KeychainDelete(service string) error { return nil }

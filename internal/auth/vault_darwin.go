//go:build darwin

package auth

import (
	"encoding/json"
	"errors"
)

// keychain item coordinates.
const (
	kcAccount = "oauth"
	kcService = "com.go-reddit.reader"
)

// keychainVault stores the OAuth credentials in the login Keychain behind a
// biometric SecAccessControl (see keychain_darwin.go). No secret ever transits
// argv; the Touch ID prompt is enforced by the Keychain on every read.
type keychainVault struct{}

// NewVault returns the platform Vault (Keychain + Touch ID on macOS).
func NewVault() Vault { return keychainVault{} }

func (keychainVault) Available() bool { return keychainLoadErr == nil }

func (keychainVault) Save(c Credentials) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return keychainStore(kcService, kcAccount, data)
}

func (keychainVault) Load() (Credentials, error) {
	data, err := keychainLoad(kcService, kcAccount)
	if errors.Is(err, errKeychainNotFound) {
		return Credentials{}, ErrNoCredentials
	}
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

func (keychainVault) Clear() error { return keychainDelete(kcService, kcAccount) }

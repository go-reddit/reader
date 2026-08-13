//go:build darwin

package auth

import (
	"encoding/json"
	"errors"

	"github.com/go-keyring/keyring"
)

// keychain item coordinates.
const (
	kcAccount = "oauth"
	kcService = "com.go-reddit.reader"
	// kcProbeAccount is a never-written account used only to check that the
	// vault backend is reachable. The lookup misses immediately (a plain
	// query for an absent item), so it returns fast and raises no biometric
	// prompt.
	kcProbeAccount = "availability-probe"
)

// Vault access seams. On a device they are the real, owned pure-Go (CGO=0)
// github.com/go-keyring/keyring calls — which on darwin dispatch to the login
// Keychain through github.com/go-macos/keychain. As package vars they also let
// the vault's marshalling and error mapping be unit-tested without a live
// Keychain (tests swap them for in-memory fakes).
var (
	kcSet    = keyring.Set
	kcGet    = keyring.Get
	kcDelete = keyring.Delete
)

// keychainVault stores the OAuth credentials in the login Keychain behind a
// biometric access control. No secret ever transits argv; the Touch ID prompt
// is enforced by the Keychain on every read. The credential plumbing lives in
// the cross-platform github.com/go-keyring/keyring façade, which on darwin maps
// keyring.WithUserPresence to a SecAccessControl (kSecAttrAccessibleWhenUnlocked-
// ThisDeviceOnly + kSecAccessControlUserPresence).
type keychainVault struct{}

// NewVault returns the platform Vault (Keychain + Touch ID on macOS).
func NewVault() Vault { return keychainVault{} }

// Available reports whether the vault backend is reachable on this host. It
// probes a never-written item, so it returns quickly and raises no biometric
// prompt; a backend that failed to load (or a platform without a vault)
// surfaces here as unavailable.
func (keychainVault) Available() bool {
	_, err := kcGet(kcService, kcProbeAccount)
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// Save stores the credentials behind an access control gated by Touch ID or the
// device passcode (kept on this device only), replacing any existing item.
func (keychainVault) Save(c Credentials) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return kcSet(kcService, kcAccount, data, keyring.WithUserPresence())
}

// Load reads and decodes the stored credentials, prompting Touch ID. It returns
// [ErrNoCredentials] when nothing is stored.
func (keychainVault) Load() (Credentials, error) {
	data, err := kcGet(kcService, kcAccount)
	if errors.Is(err, keyring.ErrNotFound) {
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

// Clear forgets any stored credentials (a missing item is not an error).
func (keychainVault) Clear() error { return kcDelete(kcService, kcAccount) }

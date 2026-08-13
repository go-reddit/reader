//go:build darwin

package auth

import (
	"encoding/json"
	"errors"

	"github.com/go-macos/keychain"
)

// keychain item coordinates.
const (
	kcAccount = "oauth"
	kcService = "com.go-reddit.reader"
	// kcProbeAccount is a never-written account used only to check that the
	// Keychain backend is reachable. The lookup misses immediately (a plain
	// generic-password query for an absent item), so it returns fast and raises
	// no biometric prompt.
	kcProbeAccount = "availability-probe"
)

// Keychain access seams. On a device they are the real, owned pure-Go (CGO=0)
// github.com/go-macos/keychain calls; as package vars they also let the vault's
// marshalling and error mapping be unit-tested without a live Keychain (tests
// swap them for in-memory fakes).
var (
	kcSet    = keychain.Set
	kcGet    = keychain.Get
	kcDelete = keychain.Delete
)

// keychainVault stores the OAuth credentials in the login Keychain behind a
// biometric SecAccessControl. No secret ever transits argv; the Touch ID prompt
// is enforced by the Keychain on every read. The Keychain plumbing lives in the
// owned github.com/go-macos/keychain package (kSecAttrAccessibleWhenUnlocked-
// ThisDeviceOnly + kSecAccessControlUserPresence, applied via WithAccessControl).
type keychainVault struct{}

// NewVault returns the platform Vault (Keychain + Touch ID on macOS).
func NewVault() Vault { return keychainVault{} }

// Available reports whether the Keychain backend is reachable on this host. It
// probes a never-written item, so it returns quickly and raises no biometric
// prompt; a backend that failed to load (or a platform without a Keychain)
// surfaces here as unavailable.
func (keychainVault) Available() bool {
	_, err := kcGet(kcService, kcProbeAccount)
	return err == nil || errors.Is(err, keychain.ErrNotFound)
}

// Save stores the credentials behind a SecAccessControl gated by Touch ID or the
// device passcode (kept on this device only), replacing any existing item.
func (keychainVault) Save(c Credentials) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return kcSet(kcService, kcAccount, data, keychain.WithAccessControl(keychain.UserPresence))
}

// Load reads and decodes the stored credentials, prompting Touch ID. It returns
// [ErrNoCredentials] when nothing is stored.
func (keychainVault) Load() (Credentials, error) {
	data, err := kcGet(kcService, kcAccount)
	if errors.Is(err, keychain.ErrNotFound) {
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

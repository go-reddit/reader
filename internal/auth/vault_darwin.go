//go:build darwin

package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/go-reddit/reader/internal/webview"
)

// keychain item coordinates.
const (
	kcAccount = "oauth"
	kcService = "com.go-reddit.reader"
)

// keychainVault stores credentials in the login Keychain, gating every read
// and write behind a Touch ID prompt (via LocalAuthentication). Storage uses
// the `security` tool; the Touch ID gate is enforced before each access.
//
// Note: `security add-generic-password -w <value>` briefly places the secret
// in this process's argv (visible to local `ps`). A future hardening pass
// should move to the SecItem CoreFoundation API with a biometry-bound
// SecAccessControl so the secret never transits argv.
type keychainVault struct{}

// NewVault returns the platform Vault (Keychain + Touch ID on macOS).
func NewVault() Vault { return keychainVault{} }

func (keychainVault) Available() bool { return true }

func (keychainVault) Save(c Credentials) error {
	if err := webview.TouchIDAuthenticate("save your Reddit login"); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	cmd := exec.Command("security", "add-generic-password",
		"-a", kcAccount, "-s", kcService, "-U", "-w", string(data))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain save: %v: %s", err, stderr.String())
	}
	return nil
}

func (keychainVault) Load() (Credentials, error) {
	if err := webview.TouchIDAuthenticate("unlock your Reddit login"); err != nil {
		return Credentials{}, err
	}
	out, err := exec.Command("security", "find-generic-password",
		"-a", kcAccount, "-s", kcService, "-w").Output()
	if err != nil {
		return Credentials{}, ErrNoCredentials
	}
	var c Credentials
	if err := json.Unmarshal(bytes.TrimSpace(out), &c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

func (keychainVault) Clear() error {
	// Best-effort: a missing item is not an error worth surfacing.
	_ = exec.Command("security", "delete-generic-password",
		"-a", kcAccount, "-s", kcService).Run()
	return nil
}

//go:build !darwin

package auth

// unavailableVault is the fallback on platforms without Touch ID / Keychain.
type unavailableVault struct{}

// NewVault returns a Vault that reports itself unavailable.
func NewVault() Vault { return unavailableVault{} }

func (unavailableVault) Available() bool            { return false }
func (unavailableVault) Save(Credentials) error     { return ErrUnavailable }
func (unavailableVault) Load() (Credentials, error) { return Credentials{}, ErrUnavailable }
func (unavailableVault) Clear() error               { return nil }

// Package auth handles Reddit OAuth login guarded by Touch ID. The user enters
// their Reddit application's client id + secret once (from
// reddit.com/prefs/apps); those are stored in the macOS Keychain behind a
// biometric gate and unlocked with Touch ID on later launches. The reddit
// client is then reconfigured to use OAuth, which works even where Reddit
// blocks anonymous ".json" access.
//
// The Service here is platform-neutral orchestration; the actual biometric +
// Keychain work lives behind the [Vault] interface (see vault_darwin.go). This
// keeps the login flow, HTTP wiring and state fully testable without a device.
package auth

import (
	"errors"
	"sync"
)

// Credentials is a Reddit OAuth application's client id + secret.
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Valid reports whether both fields are present.
func (c Credentials) Valid() bool { return c.ClientID != "" && c.ClientSecret != "" }

// ErrNoCredentials is returned by [Vault.Load] when nothing is stored.
var ErrNoCredentials = errors.New("auth: no stored credentials")

// ErrUnavailable is returned when biometrics / the keychain are not usable.
var ErrUnavailable = errors.New("auth: Touch ID / keychain unavailable")

// Vault persists credentials behind the OS keychain, gated by Touch ID.
type Vault interface {
	Available() bool            // biometrics + keychain usable on this host
	Save(Credentials) error     // prompt Touch ID, then store
	Load() (Credentials, error) // prompt Touch ID, then read ([ErrNoCredentials] if empty)
	Clear() error               // forget any stored credentials
}

// Service orchestrates login/unlock/logout and tracks whether we are currently
// authenticated. onAuth is invoked with the credentials whenever a login or
// unlock succeeds, so the caller can reconfigure the Reddit client.
type Service struct {
	vault  Vault
	onAuth func(Credentials)

	mu       sync.Mutex
	loggedIn bool
}

// NewService wires a Vault to an onAuth callback (which may be nil).
func NewService(v Vault, onAuth func(Credentials)) *Service {
	return &Service{vault: v, onAuth: onAuth}
}

// Available reports whether login is possible on this host.
func (s *Service) Available() bool { return s.vault != nil && s.vault.Available() }

// LoggedIn reports the current authentication state.
func (s *Service) LoggedIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loggedIn
}

// Login validates and stores new credentials (prompting Touch ID via the
// vault), then applies them.
func (s *Service) Login(c Credentials) error {
	if !c.Valid() {
		return errors.New("auth: client id and secret are required")
	}
	if s.vault == nil {
		return ErrUnavailable
	}
	if err := s.vault.Save(c); err != nil {
		return err
	}
	s.apply(c)
	return nil
}

// Unlock loads the stored credentials (prompting Touch ID) and applies them.
// Used by the menu-bar "Log in with Touch ID" action and at startup.
func (s *Service) Unlock() error {
	if s.vault == nil {
		return ErrUnavailable
	}
	c, err := s.vault.Load()
	if err != nil {
		return err
	}
	s.apply(c)
	return nil
}

// Logout forgets the stored credentials and drops back to anonymous access.
func (s *Service) Logout() error {
	s.mu.Lock()
	s.loggedIn = false
	s.mu.Unlock()
	if s.vault == nil {
		return nil
	}
	return s.vault.Clear()
}

func (s *Service) apply(c Credentials) {
	s.mu.Lock()
	s.loggedIn = true
	s.mu.Unlock()
	if s.onAuth != nil {
		s.onAuth(c)
	}
}

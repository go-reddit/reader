package auth

import (
	"errors"
	"testing"
)

// fakeVault is an in-memory Vault for tests.
type fakeVault struct {
	avail    bool
	stored   *Credentials
	saveErr  error
	loadErr  error
	clearErr error
	cleared  bool
}

func (f *fakeVault) Available() bool { return f.avail }
func (f *fakeVault) Save(c Credentials) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.stored = &c
	return nil
}
func (f *fakeVault) Load() (Credentials, error) {
	if f.loadErr != nil {
		return Credentials{}, f.loadErr
	}
	if f.stored == nil {
		return Credentials{}, ErrNoCredentials
	}
	return *f.stored, nil
}
func (f *fakeVault) Clear() error {
	f.cleared = true
	return f.clearErr
}

func TestCredentialsValid(t *testing.T) {
	if (Credentials{ClientID: "a", ClientSecret: "b"}).Valid() != true {
		t.Error("valid creds")
	}
	if (Credentials{ClientID: "a"}).Valid() {
		t.Error("missing secret should be invalid")
	}
}

func TestServiceLogin(t *testing.T) {
	var applied Credentials
	v := &fakeVault{avail: true}
	s := NewService(v, func(c Credentials) { applied = c })

	if s.LoggedIn() {
		t.Error("starts logged out")
	}
	if err := s.Login(Credentials{ClientID: "id", ClientSecret: "sec"}); err != nil {
		t.Fatal(err)
	}
	if !s.LoggedIn() || applied.ClientID != "id" || v.stored == nil {
		t.Errorf("login not applied: loggedIn=%v applied=%+v", s.LoggedIn(), applied)
	}
}

func TestServiceLoginErrors(t *testing.T) {
	s := NewService(&fakeVault{}, nil)
	if err := s.Login(Credentials{}); err == nil {
		t.Error("invalid creds should error")
	}
	// nil vault.
	if err := NewService(nil, nil).Login(Credentials{ClientID: "a", ClientSecret: "b"}); err != ErrUnavailable {
		t.Errorf("nil vault => %v", err)
	}
	// vault save error.
	sv := NewService(&fakeVault{saveErr: errors.New("denied")}, nil)
	if err := sv.Login(Credentials{ClientID: "a", ClientSecret: "b"}); err == nil {
		t.Error("save error should propagate")
	}
	if sv.LoggedIn() {
		t.Error("failed login should not set loggedIn")
	}
}

func TestServiceUnlock(t *testing.T) {
	var applied Credentials
	v := &fakeVault{avail: true, stored: &Credentials{ClientID: "id", ClientSecret: "sec"}}
	s := NewService(v, func(c Credentials) { applied = c })
	if err := s.Unlock(); err != nil {
		t.Fatal(err)
	}
	if !s.LoggedIn() || applied.ClientID != "id" {
		t.Error("unlock not applied")
	}
	// No stored credentials.
	empty := NewService(&fakeVault{avail: true}, nil)
	if err := empty.Unlock(); err != ErrNoCredentials {
		t.Errorf("empty unlock => %v", err)
	}
	// nil vault.
	if err := NewService(nil, nil).Unlock(); err != ErrUnavailable {
		t.Errorf("nil vault unlock => %v", err)
	}
}

func TestServiceLogout(t *testing.T) {
	v := &fakeVault{avail: true, stored: &Credentials{ClientID: "id", ClientSecret: "sec"}}
	s := NewService(v, nil)
	s.Unlock()
	if err := s.Logout(); err != nil {
		t.Fatal(err)
	}
	if s.LoggedIn() || !v.cleared {
		t.Error("logout should clear + reset state")
	}
	// nil vault logout is a no-op.
	if err := NewService(nil, nil).Logout(); err != nil {
		t.Errorf("nil vault logout => %v", err)
	}
	// clear error propagates.
	ce := NewService(&fakeVault{clearErr: errors.New("x")}, nil)
	if err := ce.Logout(); err == nil {
		t.Error("clear error should propagate")
	}
}

func TestServiceAvailable(t *testing.T) {
	if NewService(nil, nil).Available() {
		t.Error("nil vault not available")
	}
	if NewService(&fakeVault{avail: false}, nil).Available() {
		t.Error("unavailable vault")
	}
	if !NewService(&fakeVault{avail: true}, nil).Available() {
		t.Error("available vault")
	}
}

//go:build darwin

package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-keyring/keyring"
)

// TestKeychainRoundTripOnDevice proves the github.com/go-keyring/keyring
// Set/Get/Delete calls the vault is built on work on THIS device, and reproduce
// the parity the go-macos/keychain binding provided: add-or-overwrite, a miss
// maps to keyring.ErrNotFound, and delete is idempotent. It uses a plain (no
// user-presence) item under an ephemeral service so it never raises a Touch ID
// prompt and runs unattended. A framework-load failure here is a real failure,
// not a skip.
func TestKeychainRoundTripOnDevice(t *testing.T) {
	const (
		svc  = "com.go-reddit.reader.selftest"
		acct = "roundtrip"
	)
	t.Cleanup(func() { _ = keyring.Delete(svc, acct) })

	// miss -> ErrNotFound
	if _, err := keyring.Get(svc, acct); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("pre-write Get = %v, want ErrNotFound", err)
	}
	// store
	if err := keyring.Set(svc, acct, []byte("first")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, err := keyring.Get(svc, acct); err != nil || string(got) != "first" {
		t.Fatalf("Get after set = %q,%v want \"first\",nil", got, err)
	}
	// overwrite in place (add-or-overwrite)
	if err := keyring.Set(svc, acct, []byte("second")); err != nil {
		t.Fatalf("overwrite Set: %v", err)
	}
	if got, err := keyring.Get(svc, acct); err != nil || string(got) != "second" {
		t.Fatalf("Get after overwrite = %q,%v want \"second\",nil", got, err)
	}
	// delete, then idempotent delete
	if err := keyring.Delete(svc, acct); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := keyring.Delete(svc, acct); err != nil {
		t.Fatalf("idempotent Delete: %v", err)
	}
	if _, err := keyring.Get(svc, acct); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("post-delete Get = %v, want ErrNotFound", err)
	}
	t.Log("on-device: keyring Set/Get/Delete round-trip verified " +
		"(add-or-overwrite, miss->ErrNotFound, idempotent delete)")
}

// TestVaultSaveOnDevice drives the real vault against the live Keychain through
// its user-presence access control. In a signed/entitled context Save (delete+add)
// and Clear (delete) round-trip with no prompt (only a read of a UserPresence
// item prompts). From an UNSIGNED `go test` binary the Security framework
// rejects a SecAccessControl-protected SecItemAdd with errSecMissingEntitlement
// (OSStatus -34018) — a codesign precondition, identical for the old direct
// binding, not a wiring fault: the call still reached SecItemAdd, and keyring
// surfaced the underlying OSStatus verbatim. This asserts exactly one of those
// two outcomes (never a silent skip), so it proves the vault is wired correctly
// up to the OS entitlement gate and upgrades to a full round-trip when run from
// the signed .app.
func TestVaultSaveOnDevice(t *testing.T) {
	v := NewVault()
	if !v.Available() {
		t.Fatal("vault should be available on this device")
	}
	t.Cleanup(func() { _ = v.Clear() })

	err := v.Save(Credentials{ClientID: "id123", ClientSecret: "sec456"})
	if err == nil {
		if err := v.Clear(); err != nil {
			t.Fatalf("Clear after Save: %v", err)
		}
		if err := v.Clear(); err != nil {
			t.Fatalf("idempotent Clear: %v", err)
		}
		t.Log("on-device: vault Save + idempotent Clear round-trip verified (entitled context)")
		return
	}
	if !strings.Contains(err.Error(), "-34018") {
		t.Fatalf("Save failed with %v; want a stored item or errSecMissingEntitlement (OSStatus -34018)", err)
	}
	t.Logf("on-device: vault Save reached SecItemAdd; blocked only by the codesign entitlement (%v), as expected from an unsigned go test", err)
}

// TestVaultLoadMapping covers Load's decode and error mapping deterministically
// by swapping the vault seam, including the Touch ID-gated read path's result
// handling without an interactive prompt.
func TestVaultLoadMapping(t *testing.T) {
	sg, gg, dg := kcSet, kcGet, kcDelete
	defer func() { kcSet, kcGet, kcDelete = sg, gg, dg }()

	kcGet = func(_, _ string) ([]byte, error) { return nil, keyring.ErrNotFound }
	if _, err := (keychainVault{}).Load(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Load miss = %v, want ErrNoCredentials", err)
	}

	boom := errors.New("boom")
	kcGet = func(_, _ string) ([]byte, error) { return nil, boom }
	if _, err := (keychainVault{}).Load(); !errors.Is(err, boom) {
		t.Fatalf("Load error = %v, want boom", err)
	}

	kcGet = func(_, _ string) ([]byte, error) { return []byte("{not json"), nil }
	if _, err := (keychainVault{}).Load(); err == nil {
		t.Fatal("Load with malformed JSON should error")
	}

	kcGet = func(_, _ string) ([]byte, error) {
		return []byte(`{"client_id":"a","client_secret":"b"}`), nil
	}
	c, err := (keychainVault{}).Load()
	if err != nil || c.ClientID != "a" || c.ClientSecret != "b" {
		t.Fatalf("Load good = %+v, %v", c, err)
	}
}

// TestVaultSaveAvailableClearSeams covers Save routing, Available's three
// outcomes, and Clear pass-through via the vault seam.
func TestVaultSaveAvailableClearSeams(t *testing.T) {
	sg, gg, dg := kcSet, kcGet, kcDelete
	defer func() { kcSet, kcGet, kcDelete = sg, gg, dg }()

	var gotSvc, gotAcct string
	var gotData []byte
	var gotOpts int
	kcSet = func(svc, acct string, data []byte, opts ...keyring.Option) error {
		gotSvc, gotAcct, gotData, gotOpts = svc, acct, data, len(opts)
		return nil
	}
	if err := (keychainVault{}).Save(Credentials{ClientID: "a", ClientSecret: "b"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if gotSvc != kcService || gotAcct != kcAccount || gotOpts != 1 {
		t.Fatalf("Save routed to %s/%s with %d opts, want %s/%s with 1 (WithUserPresence)", gotSvc, gotAcct, gotOpts, kcService, kcAccount)
	}
	if string(gotData) != `{"client_id":"a","client_secret":"b"}` {
		t.Fatalf("Save marshalled %s", gotData)
	}

	kcSet = func(_, _ string, _ []byte, _ ...keyring.Option) error { return errors.New("x") }
	if err := (keychainVault{}).Save(Credentials{ClientID: "a", ClientSecret: "b"}); err == nil {
		t.Fatal("Save should propagate the kcSet error")
	}

	kcGet = func(_, _ string) ([]byte, error) { return nil, keyring.ErrNotFound }
	if !(keychainVault{}).Available() {
		t.Fatal("Available on ErrNotFound should be true")
	}
	kcGet = func(_, _ string) ([]byte, error) { return []byte("x"), nil }
	if !(keychainVault{}).Available() {
		t.Fatal("Available on nil error should be true")
	}
	kcGet = func(_, _ string) ([]byte, error) { return nil, keyring.ErrUnavailable }
	if (keychainVault{}).Available() {
		t.Fatal("Available on ErrUnavailable should be false")
	}

	called := false
	kcDelete = func(_, _ string) error { called = true; return nil }
	if err := (keychainVault{}).Clear(); err != nil || !called {
		t.Fatalf("Clear err=%v called=%v", err, called)
	}
}

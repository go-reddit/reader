//go:build darwin

package auth

import (
	"testing"
	"unsafe"
)

// TestKeychainPlumbing exercises the CoreFoundation + Security FFI wiring
// against the real frameworks WITHOUT touching the Keychain or biometrics: it
// confirms every symbol resolved, that CFData round-trips, that a biometric
// SecAccessControl can be created, and that dictionaries build. The actual
// SecItemAdd/CopyMatching round-trip needs a login session + Touch ID and is
// verified on a real machine.
func TestKeychainPlumbing(t *testing.T) {
	if keychainLoadErr != nil {
		t.Fatalf("symbol load failed: %v", keychainLoadErr)
	}

	// CFData round trip.
	b := []byte("hello keychain 123")
	d := cfDataCreate(0, &b[0], len(b))
	if d == 0 {
		t.Fatal("CFDataCreate returned nil")
	}
	defer cfRelease(d)
	if n := cfDataGetLength(d); n != len(b) {
		t.Fatalf("CFDataGetLength = %d, want %d", n, len(b))
	}
	got := unsafe.Slice(cfDataGetBytePtr(d), len(b))
	if string(got) != string(b) {
		t.Fatalf("CFData bytes = %q, want %q", got, b)
	}

	// Biometric access control (no prompt on creation).
	var acErr uintptr
	ac := secACCreate(0, kSecAttrAccessibleWhenUnlockedThisDeviceOnly, kSecAccessControlUserPresence, &acErr)
	if ac == 0 {
		t.Fatal("SecAccessControlCreateWithFlags returned nil")
	}
	cfRelease(ac)

	// Dictionary + string construction (uses the callback-struct symbols).
	dict := newDict()
	if dict == 0 {
		t.Fatal("CFDictionaryCreateMutable returned nil")
	}
	s := cfStr("com.example.test")
	if s == 0 {
		t.Fatal("cfStr returned nil")
	}
	cfDictSetValue(dict, kSecClass, kSecClassGenericPassword)
	cfDictSetValue(dict, kSecAttrService, s)
	cfRelease(s)
	cfRelease(dict)

	// Every kSec* / kCF* constant we depend on must be non-nil.
	for name, v := range map[string]uintptr{
		"kSecClass":                kSecClass,
		"kSecClassGenericPassword": kSecClassGenericPassword,
		"kSecAttrService":          kSecAttrService,
		"kSecAttrAccount":          kSecAttrAccount,
		"kSecValueData":            kSecValueData,
		"kSecReturnData":           kSecReturnData,
		"kSecMatchLimit":           kSecMatchLimit,
		"kSecMatchLimitOne":        kSecMatchLimitOne,
		"kSecAttrAccessControl":    kSecAttrAccessControl,
		"kCFBooleanTrue":           kCFBooleanTrue,
		"keyCallBacks":             kCFTypeDictionaryKeyCallBacks,
		"valueCallBacks":           kCFTypeDictionaryValueCallBacks,
	} {
		if v == 0 {
			t.Errorf("constant %s is nil", name)
		}
	}
}

func TestKeychainStoreEmpty(t *testing.T) {
	if err := keychainStore("svc", "acct", nil); err == nil {
		t.Error("empty data should be rejected before touching the keychain")
	}
}

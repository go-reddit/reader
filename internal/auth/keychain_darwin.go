//go:build darwin

package auth

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// This file binds the CoreFoundation + Security C APIs through purego (CGO=0)
// to store the OAuth credentials in the login Keychain behind a biometric
// SecAccessControl. Compared with shelling out to `security -w <secret>`, the
// secret never appears in this process's argv, and the Touch ID gate is
// enforced by the Keychain item itself on every read (SecItemCopyMatching).

// CoreFoundation constants.
const (
	kCFStringEncodingUTF8         = 0x08000100
	kSecAccessControlUserPresence = 1 << 0 // Touch ID or device passcode
)

// OSStatus values we care about.
const (
	errSecSuccess      = 0
	errSecItemNotFound = -25300
)

// errKeychainNotFound signals an absent item (mapped to ErrNoCredentials).
var errKeychainNotFound = errors.New("keychain: item not found")

var (
	cfStringCreate      func(alloc uintptr, s string, enc uint32) uintptr
	cfDataCreate        func(alloc uintptr, bytes *byte, length int) uintptr
	cfDictCreateMutable func(alloc uintptr, capacity int, keyCB, valCB uintptr) uintptr
	cfDictSetValue      func(dict, key, value uintptr)
	cfRelease           func(cf uintptr)
	cfDataGetLength     func(data uintptr) int
	cfDataGetBytePtr    func(data uintptr) *byte

	secItemAdd          func(attrs uintptr, result *uintptr) int32
	secItemCopyMatching func(query uintptr, result *uintptr) int32
	secItemDelete       func(query uintptr) int32
	secACCreate         func(alloc, protection uintptr, flags uint, err *uintptr) uintptr

	// CFStringRef / CFBooleanRef constants (the symbol holds the ref value).
	kSecClass                                  uintptr
	kSecClassGenericPassword                   uintptr
	kSecAttrService                            uintptr
	kSecAttrAccount                            uintptr
	kSecValueData                              uintptr
	kSecReturnData                             uintptr
	kSecMatchLimit                             uintptr
	kSecMatchLimitOne                          uintptr
	kSecAttrAccessControl                      uintptr
	kSecAttrAccessibleWhenUnlockedThisDeviceOnly uintptr
	kCFBooleanTrue                             uintptr

	// Addresses of the dictionary callback structs (passed by pointer).
	kCFTypeDictionaryKeyCallBacks   uintptr
	kCFTypeDictionaryValueCallBacks uintptr

	keychainLoadErr error
)

func init() { loadKeychain() }

func loadKeychain() {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		keychainLoadErr = fmt.Errorf("keychain: load CoreFoundation: %w", err)
		return
	}
	sec, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		keychainLoadErr = fmt.Errorf("keychain: load Security: %w", err)
		return
	}

	bind := func(fptr any, h uintptr, name string) {
		if keychainLoadErr != nil {
			return
		}
		p, err := purego.Dlsym(h, name)
		if err != nil || p == 0 {
			keychainLoadErr = fmt.Errorf("keychain: missing function %s", name)
			return
		}
		purego.RegisterFunc(fptr, p)
	}
	// value symbol -> dereferenced ref; struct symbol -> its address.
	ref := func(h uintptr, name string) uintptr {
		if keychainLoadErr != nil {
			return 0
		}
		p, err := purego.Dlsym(h, name)
		if err != nil || p == 0 {
			keychainLoadErr = fmt.Errorf("keychain: missing symbol %s", name)
			return 0
		}
		return *(*uintptr)(unsafe.Pointer(p))
	}
	addr := func(h uintptr, name string) uintptr {
		if keychainLoadErr != nil {
			return 0
		}
		p, err := purego.Dlsym(h, name)
		if err != nil || p == 0 {
			keychainLoadErr = fmt.Errorf("keychain: missing struct %s", name)
			return 0
		}
		return p
	}

	bind(&cfStringCreate, cf, "CFStringCreateWithCString")
	bind(&cfDataCreate, cf, "CFDataCreate")
	bind(&cfDictCreateMutable, cf, "CFDictionaryCreateMutable")
	bind(&cfDictSetValue, cf, "CFDictionarySetValue")
	bind(&cfRelease, cf, "CFRelease")
	bind(&cfDataGetLength, cf, "CFDataGetLength")
	bind(&cfDataGetBytePtr, cf, "CFDataGetBytePtr")
	bind(&secItemAdd, sec, "SecItemAdd")
	bind(&secItemCopyMatching, sec, "SecItemCopyMatching")
	bind(&secItemDelete, sec, "SecItemDelete")
	bind(&secACCreate, sec, "SecAccessControlCreateWithFlags")

	kCFBooleanTrue = ref(cf, "kCFBooleanTrue")
	kCFTypeDictionaryKeyCallBacks = addr(cf, "kCFTypeDictionaryKeyCallBacks")
	kCFTypeDictionaryValueCallBacks = addr(cf, "kCFTypeDictionaryValueCallBacks")
	kSecClass = ref(sec, "kSecClass")
	kSecClassGenericPassword = ref(sec, "kSecClassGenericPassword")
	kSecAttrService = ref(sec, "kSecAttrService")
	kSecAttrAccount = ref(sec, "kSecAttrAccount")
	kSecValueData = ref(sec, "kSecValueData")
	kSecReturnData = ref(sec, "kSecReturnData")
	kSecMatchLimit = ref(sec, "kSecMatchLimit")
	kSecMatchLimitOne = ref(sec, "kSecMatchLimitOne")
	kSecAttrAccessControl = ref(sec, "kSecAttrAccessControl")
	kSecAttrAccessibleWhenUnlockedThisDeviceOnly = ref(sec, "kSecAttrAccessibleWhenUnlockedThisDeviceOnly")
}

// cfStr makes a CFString from a Go string (caller releases).
func cfStr(s string) uintptr { return cfStringCreate(0, s, kCFStringEncodingUTF8) }

// newDict makes an empty mutable CFDictionary (caller releases).
func newDict() uintptr {
	return cfDictCreateMutable(0, 0, kCFTypeDictionaryKeyCallBacks, kCFTypeDictionaryValueCallBacks)
}

// keychainStore writes data for (service, account) with a biometric access
// control, replacing any existing item.
func keychainStore(service, account string, data []byte) error {
	if keychainLoadErr != nil {
		return keychainLoadErr
	}
	if len(data) == 0 {
		return errors.New("keychain: empty data")
	}
	svc, acc := cfStr(service), cfStr(account)
	defer cfRelease(svc)
	defer cfRelease(acc)

	// Remove any prior item so add doesn't hit errSecDuplicateItem.
	del := newDict()
	cfDictSetValue(del, kSecClass, kSecClassGenericPassword)
	cfDictSetValue(del, kSecAttrService, svc)
	cfDictSetValue(del, kSecAttrAccount, acc)
	secItemDelete(del)
	cfRelease(del)

	var acErr uintptr
	ac := secACCreate(0, kSecAttrAccessibleWhenUnlockedThisDeviceOnly, kSecAccessControlUserPresence, &acErr)
	if ac == 0 {
		return errors.New("keychain: SecAccessControlCreateWithFlags failed")
	}
	defer cfRelease(ac)

	d := cfDataCreate(0, &data[0], len(data))
	defer cfRelease(d)

	attrs := newDict()
	defer cfRelease(attrs)
	cfDictSetValue(attrs, kSecClass, kSecClassGenericPassword)
	cfDictSetValue(attrs, kSecAttrService, svc)
	cfDictSetValue(attrs, kSecAttrAccount, acc)
	cfDictSetValue(attrs, kSecValueData, d)
	cfDictSetValue(attrs, kSecAttrAccessControl, ac)

	if st := secItemAdd(attrs, nil); st != errSecSuccess {
		return fmt.Errorf("keychain: SecItemAdd OSStatus %d", st)
	}
	return nil
}

// keychainLoad reads the data for (service, account). Reading a biometric item
// triggers the Touch ID prompt. Returns [errKeychainNotFound] when absent.
func keychainLoad(service, account string) ([]byte, error) {
	if keychainLoadErr != nil {
		return nil, keychainLoadErr
	}
	svc, acc := cfStr(service), cfStr(account)
	defer cfRelease(svc)
	defer cfRelease(acc)

	q := newDict()
	defer cfRelease(q)
	cfDictSetValue(q, kSecClass, kSecClassGenericPassword)
	cfDictSetValue(q, kSecAttrService, svc)
	cfDictSetValue(q, kSecAttrAccount, acc)
	cfDictSetValue(q, kSecReturnData, kCFBooleanTrue)
	cfDictSetValue(q, kSecMatchLimit, kSecMatchLimitOne)

	var result uintptr
	switch st := secItemCopyMatching(q, &result); st {
	case errSecSuccess:
	case errSecItemNotFound:
		return nil, errKeychainNotFound
	default:
		return nil, fmt.Errorf("keychain: SecItemCopyMatching OSStatus %d", st)
	}
	defer cfRelease(result)

	n := cfDataGetLength(result)
	if n <= 0 {
		return nil, errKeychainNotFound
	}
	p := cfDataGetBytePtr(result)
	out := make([]byte, n)
	copy(out, unsafe.Slice(p, n))
	return out, nil
}

// keychainDelete removes the item (a missing item is not an error).
func keychainDelete(service, account string) error {
	if keychainLoadErr != nil {
		return keychainLoadErr
	}
	svc, acc := cfStr(service), cfStr(account)
	defer cfRelease(svc)
	defer cfRelease(acc)
	q := newDict()
	defer cfRelease(q)
	cfDictSetValue(q, kSecClass, kSecClassGenericPassword)
	cfDictSetValue(q, kSecAttrService, svc)
	cfDictSetValue(q, kSecAttrAccount, acc)
	secItemDelete(q)
	return nil
}

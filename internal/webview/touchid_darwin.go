// Touch ID via the LocalAuthentication framework, driven through the
// Objective-C runtime with purego (CGO=0). Used by the auth package to gate
// Keychain access behind biometrics.
//
//go:build darwin

package webview

import (
	"errors"
	"fmt"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	selCanEvalPolicy = objc.RegisterName("canEvaluatePolicy:error:")
	selEvalPolicy    = objc.RegisterName("evaluatePolicy:localizedReason:reply:")
)

// LAPolicyDeviceOwnerAuthenticationWithBiometrics.
const laPolicyBiometrics = 1

var laLoaded bool

// TouchIDAuthenticate presents the system Touch ID prompt with the given
// reason and blocks until the user approves, cancels, or it times out. It
// returns nil on success. Requires a machine with biometrics enrolled.
func TouchIDAuthenticate(reason string) error {
	if err := loadFrameworks(); err != nil {
		return err
	}
	if !laLoaded {
		if _, err := purego.Dlopen(
			"/System/Library/Frameworks/LocalAuthentication.framework/LocalAuthentication",
			purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
			return fmt.Errorf("touchid: load framework: %w", err)
		}
		laLoaded = true
	}
	cls := objc.GetClass("LAContext")
	if cls == 0 {
		return errors.New("touchid: LAContext unavailable")
	}
	ctx := objc.ID(cls).Send(selAlloc).Send(selInit)
	if ctx.Send(selCanEvalPolicy, laPolicyBiometrics, uintptr(0)) == 0 {
		return errors.New("touchid: biometrics not available or not enrolled")
	}

	done := make(chan bool, 1)
	block := objc.NewBlock(func(_ objc.Block, success bool, _ objc.ID) { done <- success })
	ctx.Send(selEvalPolicy, laPolicyBiometrics, nsString(reason), block)

	select {
	case ok := <-done:
		if !ok {
			return errors.New("touchid: authentication failed or cancelled")
		}
		return nil
	case <-time.After(60 * time.Second):
		return errors.New("touchid: timed out")
	}
}

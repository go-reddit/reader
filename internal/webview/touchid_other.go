//go:build !darwin

package webview

import "errors"

// TouchIDAuthenticate is unavailable off macOS.
func TouchIDAuthenticate(reason string) error {
	return errors.New("touchid: only available on macOS")
}

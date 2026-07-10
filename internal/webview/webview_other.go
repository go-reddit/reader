// On non-macOS platforms there is no WKWebView; Run reports that the native
// window is unavailable so the caller can fall back to opening a browser.
//
//go:build !darwin

package webview

import "errors"

// ErrUnsupported is returned by [Run] on platforms without a native webview.
var ErrUnsupported = errors.New("webview: native window is only available on macOS")

// Config controls the hosted window (fields mirror the darwin build).
type Config struct {
	Title  string
	URL    string
	Width  float64
	Height float64
}

// Run is unavailable off macOS and always returns [ErrUnsupported].
func Run(cfg Config) error { return ErrUnsupported }

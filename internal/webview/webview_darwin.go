// Package webview opens a native macOS window hosting a WKWebView, pointed at
// a URL. It drives the Cocoa/WebKit frameworks entirely through the
// Objective-C runtime via github.com/ebitengine/purego — no cgo, so the whole
// app builds and links with CGO_ENABLED=0 (the fleet-wide requirement).
//
// The bridge is deliberately minimal: one borderless-titled window, one
// WKWebView filling it, and the standard NSApplication run loop. It is the
// "chrome" around the wasm UI; everything the user sees inside the window is
// drawn by internal/ui.
//
//go:build darwin

package webview

import (
	"fmt"
	"net/http"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// selectors and classes are resolved once at first use.
var (
	selAlloc                      = objc.RegisterName("alloc")
	selInit                       = objc.RegisterName("init")
	selRetain                     = objc.RegisterName("retain")
	selSharedApplication          = objc.RegisterName("sharedApplication")
	selSetActivationPolicy        = objc.RegisterName("setActivationPolicy:")
	selActivateIgnoringOtherApps  = objc.RegisterName("activateIgnoringOtherApps:")
	selRun                        = objc.RegisterName("run")
	selStringWithUTF8String       = objc.RegisterName("stringWithUTF8String:")
	selURLWithString              = objc.RegisterName("URLWithString:")
	selRequestWithURL             = objc.RegisterName("requestWithURL:")
	selLoadRequest                = objc.RegisterName("loadRequest:")
	selSetTitle                   = objc.RegisterName("setTitle:")
	selSetContentView             = objc.RegisterName("setContentView:")
	selMakeKeyAndOrderFront       = objc.RegisterName("makeKeyAndOrderFront:")
	selCenter                     = objc.RegisterName("center")
	selInitWithContentRect        = objc.RegisterName("initWithContentRect:styleMask:backing:defer:")
	selInitWithFrameConfiguration = objc.RegisterName("initWithFrame:configuration:")
	selSetAutoresizingMask        = objc.RegisterName("setAutoresizingMask:")
)

// NSViewAutoresizingMask bits: the web view grows in both dimensions with its
// window so the wasm canvas (which fills the view) receives resize events.
const (
	viewWidthSizable  = 1 << 1
	viewHeightSizable = 1 << 4
)

// NSWindowStyleMask bits.
const (
	styleTitled         = 1 << 0
	styleClosable       = 1 << 1
	styleMiniaturizable = 1 << 2
	styleResizable      = 1 << 3
)

const (
	backingStoreBuffered  = 2
	activationPolicyReg   = 0 // NSApplicationActivationPolicyRegular
)

// cgRect mirrors CoreGraphics CGRect (two CGFloat points + two CGFloat sizes).
// Passed by value to initWithContentRect:… ; the amd64/arm64 calling
// conventions purego implements marshal it correctly as four float64s.
type cgRect struct {
	X, Y, W, H float64
}

// Config controls the hosted window.
//
// Two transports are supported:
//   - Handler set: content is served in-process over a private URL scheme
//     (no socket). URL is the initial page path under that scheme's origin,
//     e.g. "/?sr=golang"; Scheme names the scheme (default "reader").
//   - Handler nil: URL is loaded directly (an http://127.0.0.1:PORT address).
type Config struct {
	Title   string
	URL     string
	Width   float64
	Height  float64
	Handler http.Handler // in-process request handler (socketless transport)
	Scheme  string       // custom scheme when Handler != nil (default "reader")
}

// frameworksLoaded ensures the AppKit/WebKit classes are registered exactly
// once (dlopen is idempotent but we avoid repeat work).
var frameworksLoaded bool

func loadFrameworks() error {
	if frameworksLoaded {
		return nil
	}
	for _, p := range []string{
		"/System/Library/Frameworks/Foundation.framework/Foundation",
		"/System/Library/Frameworks/AppKit.framework/AppKit",
		"/System/Library/Frameworks/WebKit.framework/WebKit",
	} {
		if _, err := purego.Dlopen(p, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
			return fmt.Errorf("webview: dlopen %s: %w", p, err)
		}
	}
	frameworksLoaded = true
	return nil
}

// nsString builds an NSString from a Go string.
func nsString(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(selStringWithUTF8String, s)
}

// Run opens the window and enters the Cocoa run loop. It blocks until the
// application terminates (the user closes the window / quits), so callers
// typically run it on the main goroutine after starting their server. It must
// run on the main OS thread — see [RunMain].
func Run(cfg Config) error {
	if err := loadFrameworks(); err != nil {
		return err
	}
	if cfg.Width == 0 {
		cfg.Width = 900
	}
	if cfg.Height == 0 {
		cfg.Height = 660
	}

	app := objc.ID(objc.GetClass("NSApplication")).Send(selSharedApplication)
	app.Send(selSetActivationPolicy, activationPolicyReg)

	rect := cgRect{X: 0, Y: 0, W: cfg.Width, H: cfg.Height}
	style := uint(styleTitled | styleClosable | styleMiniaturizable | styleResizable)

	win := objc.ID(objc.GetClass("NSWindow")).Send(selAlloc).
		Send(selInitWithContentRect, rect, style, backingStoreBuffered, false)
	win.Send(selRetain) // keep it alive past this frame

	cfgObj := objc.ID(objc.GetClass("WKWebViewConfiguration")).Send(selAlloc).Send(selInit)

	// Choose the transport: an in-process scheme handler (no socket) when a
	// Handler is supplied, otherwise a plain http(s) load.
	loadURL := cfg.URL
	if cfg.Handler != nil {
		scheme := cfg.Scheme
		if scheme == "" {
			scheme = "reader"
		}
		h, err := newSchemeHandler(cfg.Handler)
		if err != nil {
			return err
		}
		cfgObj.Send(selSetURLSchemeFor, h, nsString(scheme))
		loadURL = scheme + "://app" + normalisePath(cfg.URL)
	}

	webview := objc.ID(objc.GetClass("WKWebView")).Send(selAlloc).
		Send(selInitWithFrameConfiguration, rect, cfgObj)
	webview.Send(selSetAutoresizingMask, uint(viewWidthSizable|viewHeightSizable))

	nsurl := objc.ID(objc.GetClass("NSURL")).Send(selURLWithString, nsString(loadURL))
	req := objc.ID(objc.GetClass("NSURLRequest")).Send(selRequestWithURL, nsurl)
	webview.Send(selLoadRequest, req)

	win.Send(selSetContentView, webview)
	win.Send(selSetTitle, nsString(cfg.Title))
	win.Send(selCenter)
	win.Send(selMakeKeyAndOrderFront, objc.ID(0))

	app.Send(selActivateIgnoringOtherApps, true)
	app.Send(selRun) // blocks until the app quits
	return nil
}

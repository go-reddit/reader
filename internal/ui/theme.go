package ui

import "github.com/go-widgets/toolkit"

// OS-adaptive look & feel. The reader picks a palette that matches the host
// platform so the wasm UI feels native: macOS uses the toolkit's WhiteSur
// (Big Sur) theme, GNOME/Linux an Adwaita palette, Windows a Fluent palette.
// The front-end detects the OS from the browser and passes one of the tokens
// below; light/dark follows the system (or the user's explicit choice).

// OS tokens understood by [ThemeFor]. The front-end derives these from
// navigator.platform / userAgentData.
const (
	OSMac     = "mac"
	OSLinux   = "linux"
	OSWindows = "windows"
)

// ThemeFor returns the palette matching os (one of the OS* tokens) in the
// requested light/dark variant. Unknown platforms fall back to the toolkit
// default theme.
func ThemeFor(os string, dark bool) *toolkit.Theme {
	switch os {
	case OSMac:
		if dark {
			return toolkit.WhiteSurDark()
		}
		return toolkit.WhiteSurLight()
	case OSLinux:
		if dark {
			return toolkit.AdwaitaDark()
		}
		return toolkit.AdwaitaLight()
	case OSWindows:
		if dark {
			return toolkit.FluentDark()
		}
		return toolkit.FluentLight()
	default:
		if dark {
			return toolkit.DefaultDark()
		}
		return toolkit.DefaultLight()
	}
}

// ResolveTheme maps a persisted theme name ("system"|"light"|"dark") plus the
// host OS and the system dark preference to a concrete palette.
func ResolveTheme(name, os string, prefersDark bool) *toolkit.Theme {
	switch name {
	case "light":
		return ThemeFor(os, false)
	case "dark":
		return ThemeFor(os, true)
	default: // "system"
		return ThemeFor(os, prefersDark)
	}
}

// rgb builds an opaque colour from a 0xRRGGBB literal.
func rgb(v uint32) toolkit.RGBA {
	return toolkit.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}

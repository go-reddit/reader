package webview

import "testing"

func TestSchemePathAndQuery(t *testing.T) {
	cases := map[string]string{
		"reader://app/api/feed?sr=golang": "/api/feed?sr=golang",
		"reader://app/":                   "/",
		"reader://app":                    "/",
		"reader://app/wasm_exec.js":       "/wasm_exec.js",
		"://bad url\x7f":                  "/",
	}
	for in, want := range cases {
		if got := schemePathAndQuery(in); got != want {
			t.Errorf("schemePathAndQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalisePath(t *testing.T) {
	cases := map[string]string{"": "/", "/x": "/x", "x": "/x", "/?a=b": "/?a=b"}
	for in, want := range cases {
		if got := normalisePath(in); got != want {
			t.Errorf("normalisePath(%q) = %q, want %q", in, got, want)
		}
	}
}

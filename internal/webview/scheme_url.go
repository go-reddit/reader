package webview

import (
	"net/url"
	"strings"
)

// normalisePath ensures p is an absolute request path ("/" prefix) for use as
// the tail of a "scheme://app<path>" URL. An empty path becomes "/".
func normalisePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// schemePathAndQuery reduces a custom-scheme request URL such as
// "reader://app/api/feed?sr=golang" to the path+query an http.Handler sees:
// "/api/feed?sr=golang". A bare host ("reader://app") maps to "/". Malformed
// input falls back to "/".
func schemePathAndQuery(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "/"
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		return path + "?" + u.RawQuery
	}
	return path
}

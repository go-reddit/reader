//go:build !darwin

package webview

import "errors"

// RedditFetch is unavailable off macOS (there is no WKWebView engine).
func RedditFetch(pathAndQuery string) (string, error) {
	return "", errors.New("reddit engine: only available on macOS")
}

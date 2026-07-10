// Package browserhttp builds a portable, pure-Go (CGO=0) http.Client that
// presents a real Chrome TLS fingerprint via uTLS. Reddit's anti-bot 403 for
// non-browser clients keys largely on the TLS ClientHello (Go's default
// handshake is trivially distinguishable from a browser's); mimicking Chrome's
// ciphers/extensions/curves — plus a browser User-Agent and a warmed cookie
// jar — lets a plain Go client read the public ".json" endpoints without any
// host web view. It works identically on macOS, Linux and Windows.
package browserhttp

import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"

	utls "github.com/refraction-networking/utls"
)

// NewClient returns an http.Client whose TLS handshakes impersonate Chrome and
// which keeps cookies across requests.
func NewClient(timeout time.Duration) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout:   timeout,
		Jar:       jar,
		Transport: NewTransport(),
	}
}

// NewTransport returns an http.Transport that dials TLS with a Chrome
// fingerprint (forced to HTTP/1.1 so net/http drives the connection).
func NewTransport() *http.Transport {
	return &http.Transport{
		DialTLSContext:      dialChromeTLS,
		MaxIdleConns:        20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}
}

// dialChromeTLS dials addr and completes a uTLS handshake presenting Chrome's
// ClientHello, with ALPN pinned to http/1.1.
func dialChromeTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	d := &net.Dialer{Timeout: 15 * time.Second}
	raw, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	// Start from Chrome's fingerprint, then pin ALPN to http/1.1 so net/http
	// (which speaks HTTP/1.1 over this conn) and the negotiated protocol agree.
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
	if err != nil {
		raw.Close()
		return nil, err
	}
	for _, ext := range spec.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
		}
	}

	uconn := utls.UClient(raw, &utls.Config{ServerName: host}, utls.HelloCustom)
	if err := uconn.ApplyPreset(&spec); err != nil {
		raw.Close()
		return nil, err
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	return uconn, nil
}

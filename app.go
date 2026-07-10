package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-reddit/reader/internal/browserhttp"
	"github.com/go-reddit/reddit"
)

// options are the reader's runtime settings, populated from flags + env.
type options struct {
	subreddit string // "" => front page
	sort      string
	userAgent string

	oauthID     string
	oauthSecret string
	oauthUser   string
	oauthPass   string

	noWindow bool // force the browser-launch fallback
}

// optionsFromEnv seeds an options from environment variables. Flags in main
// override these. OAuth credentials belong in the environment (never flags),
// so they don't leak into the process table.
// defaultUserAgent makes anonymous traffic look like a real browser. Reddit
// 403s generic / library User-Agents even on residential IPs; a Safari string
// (plus a warmed cookie jar) is what an actual browser sends to read the public
// ".json" endpoints without logging in. Override with READER_USER_AGENT.
const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15"

func optionsFromEnv(getenv func(string) string) options {
	return options{
		subreddit:   getenv("READER_SUBREDDIT"),
		sort:        firstNonEmpty(getenv("READER_SORT"), "hot"),
		userAgent:   firstNonEmpty(getenv("READER_USER_AGENT"), defaultUserAgent),
		oauthID:     getenv("READER_OAUTH_CLIENT_ID"),
		oauthSecret: getenv("READER_OAUTH_CLIENT_SECRET"),
		oauthUser:   getenv("READER_OAUTH_USERNAME"),
		oauthPass:   getenv("READER_OAUTH_PASSWORD"),
	}
}

// newClient builds a reddit client from the options: anonymous by default,
// app-only OAuth when a client id+secret are present, or the script grant
// when a username+password are also supplied.
func (o options) newClient() *reddit.Client {
	opts := []reddit.Option{}
	if o.userAgent != "" {
		opts = append(opts, reddit.WithUserAgent(o.userAgent))
	}
	switch {
	case o.oauthID != "" && o.oauthSecret != "" && o.oauthUser != "" && o.oauthPass != "":
		opts = append(opts, reddit.WithOAuthScript(o.oauthID, o.oauthSecret, o.oauthUser, o.oauthPass))
		return reddit.NewClient(opts...)
	case o.oauthID != "" && o.oauthSecret != "":
		opts = append(opts, reddit.WithOAuth(o.oauthID, o.oauthSecret))
		return reddit.NewClient(opts...)
	}
	// Anonymous: a portable browser-fingerprint (uTLS Chrome) client — pure
	// Go, CGO=0, no host web view — plus a warmed cookie jar. This presents
	// the same TLS ClientHello a real Chrome does, which is the main signal
	// Reddit's anti-bot uses to 403 non-browser clients.
	hc := browserhttp.NewClient(30 * time.Second)
	opts = append(opts, reddit.WithHTTPClient(hc))
	if warmupOnStartup {
		go warmupCookies(hc, o.userAgent)
	}
	return reddit.NewClient(opts...)
}

// warmupOnStartup gates the cookie warm-up so unit tests stay network-free;
// main sets it true.
var warmupOnStartup = false

// warmupCookies loads the Reddit home page once so the shared jar collects the
// cookies Reddit hands a browser; failures are ignored (cookies are often set
// even on a 403 response).
func warmupCookies(hc *http.Client, ua string) {
	req, err := http.NewRequest(http.MethodGet, "https://www.reddit.com/", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if resp, err := hc.Do(req); err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// oauthClientFor builds an OAuth-authenticated Reddit client from stored
// credentials, preserving the configured User-Agent. Used to swap the server's
// client after a successful Touch-ID login.
func (o options) oauthClientFor(clientID, clientSecret string) *reddit.Client {
	opts := []reddit.Option{reddit.WithOAuth(clientID, clientSecret)}
	if o.userAgent != "" {
		opts = append(opts, reddit.WithUserAgent(o.userAgent))
	}
	return reddit.NewClient(opts...)
}

// feedURL builds the page URL the webview loads, carrying the initial feed
// selection as query parameters the wasm reads on boot.
func feedURL(base string, o options) string {
	base = strings.TrimRight(base, "/")
	q := url.Values{}
	if sub := strings.TrimPrefix(strings.TrimSpace(o.subreddit), "r/"); sub != "" {
		q.Set("sr", sub)
	}
	if o.sort != "" {
		q.Set("sort", o.sort)
	}
	if enc := q.Encode(); enc != "" {
		return base + "/?" + enc
	}
	return base + "/"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// getenv is a package var so tests can substitute a fake environment.
var getenv = os.Getenv

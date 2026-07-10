package main

import (
	"net/url"
	"os"
	"strings"

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
func optionsFromEnv(getenv func(string) string) options {
	return options{
		subreddit:   getenv("READER_SUBREDDIT"),
		sort:        firstNonEmpty(getenv("READER_SORT"), "hot"),
		userAgent:   getenv("READER_USER_AGENT"),
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
	case o.oauthID != "" && o.oauthSecret != "":
		opts = append(opts, reddit.WithOAuth(o.oauthID, o.oauthSecret))
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

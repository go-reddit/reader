package main

import (
	"testing"
)

func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestOptionsFromEnv(t *testing.T) {
	o := optionsFromEnv(fakeEnv(map[string]string{
		"READER_SUBREDDIT": "golang",
		"READER_SORT":      "top",
		"READER_USER_AGENT": "ua/1",
	}))
	if o.subreddit != "golang" || o.sort != "top" || o.userAgent != "ua/1" {
		t.Errorf("options = %+v", o)
	}
	// Empty sort defaults to hot.
	o = optionsFromEnv(fakeEnv(nil))
	if o.sort != "hot" {
		t.Errorf("default sort = %q, want hot", o.sort)
	}
}

func TestNewClientVariants(t *testing.T) {
	// Anonymous (no creds) — just must not panic and must return a client.
	if c := (options{userAgent: "ua"}).newClient(); c == nil {
		t.Fatal("nil client")
	}
	// App-only OAuth.
	if c := (options{oauthID: "id", oauthSecret: "sec"}).newClient(); c == nil {
		t.Fatal("nil oauth client")
	}
	// Script OAuth.
	if c := (options{oauthID: "id", oauthSecret: "sec", oauthUser: "u", oauthPass: "p"}).newClient(); c == nil {
		t.Fatal("nil script client")
	}
	// Partial creds fall back to anonymous (id without secret).
	if c := (options{oauthID: "id"}).newClient(); c == nil {
		t.Fatal("nil client for partial creds")
	}
}

func TestFeedURL(t *testing.T) {
	cases := []struct {
		base string
		o    options
		want string
	}{
		{"http://127.0.0.1:8080/", options{subreddit: "r/golang", sort: "top"}, "http://127.0.0.1:8080/?sort=top&sr=golang"},
		{"http://x", options{sort: "hot"}, "http://x/?sort=hot"},
		{"http://x", options{}, "http://x/"},
		{"http://x", options{subreddit: "  "}, "http://x/"},
	}
	for _, c := range cases {
		if got := feedURL(c.base, c.o); got != c.want {
			t.Errorf("feedURL(%q, %+v) = %q, want %q", c.base, c.o, got, c.want)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "x", "y"); got != "x" {
		t.Errorf("= %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("= %q, want empty", got)
	}
}

func TestOpenBrowserBadCommandNonFatal(t *testing.T) {
	// A bogus URL/command must not panic; the fallback print path runs.
	openBrowser("http://127.0.0.1:0/")
}

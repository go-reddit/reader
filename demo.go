package main

import (
	"context"
	"fmt"

	"github.com/go-reddit/reddit"
)

// demoFetcher implements server.Fetcher with per-subreddit sample content so
// the app (and the sidebar's feed switching) can be demonstrated and verified
// without Reddit credentials or network access. Enabled by the -demo flag.
type demoFetcher struct{}

// demoContent maps a subreddit to a handful of themed post titles+domains, so
// clicking a different bookmark visibly changes the feed. "" is the front page.
var demoContent = map[string][]demoItem{
	"": {
		{"Go 1.26 is released — a new pure-Go wasm pipeline and faster GC", "go.dev", "News"},
		{"Ask reddit: what desktop app surprised you by being written in Go?", "self.AskReddit", ""},
		{"The Rust compiler now bootstraps 12% faster after MIR changes", "blog.rust-lang.org", ""},
		{"Show HN: a native macOS Reddit reader with zero cgo", "github.com", "Show"},
		{"TIL you can drive WKWebView from Go without cgo via the ObjC runtime", "developer.apple.com", "TIL"},
	},
	"golang": {
		{"Go 1.26 is released — generics tweaks, faster GC, pure-Go wasm", "go.dev", "News"},
		{"Why I stopped reaching for CGO on macOS system integration", "blog.example.com", ""},
		{"Show reddit: go-widgets — 62 pixel-blitting widgets, 100% covered", "github.com", "Show"},
		{"Benchmarking pure-Go SIMD across all six 64-bit architectures", "self.golang", "Benchmark"},
		{"A tiny embeddable HTTP proxy pattern for wasm front-ends", "gist.github.com", ""},
	},
	"rust": {
		{"Announcing Rust 1.90 and the 2027 edition roadmap", "blog.rust-lang.org", "News"},
		{"Writing a borrow-checker-friendly ECS from scratch", "self.rust", ""},
		{"tokio 2.0: what changed and how to migrate", "tokio.rs", ""},
		{"Show: a no_std GUI toolkit that renders to a framebuffer", "github.com", "Show"},
		{"Why async traits finally feel good in 2026", "blog.example.com", ""},
	},
	"programming": {
		{"The best code review comment I ever received", "self.programming", ""},
		{"How databases handle concurrent writes: MVCC explained", "blog.example.com", ""},
		{"Debugging a heisenbug that only happened on Tuesdays", "self.programming", ""},
		{"A visual guide to how CPUs speculate", "cpu.land", "Article"},
	},
	"webdev": {
		{"Baseline 2026: the web platform features you can finally use", "web.dev", "News"},
		{"I replaced 30k lines of JS with 200 lines of wasm", "self.webdev", "Show"},
		{"CSS :has() patterns that removed all my JavaScript", "css-tricks.com", ""},
		{"Why your Lighthouse score lies to you", "blog.example.com", ""},
	},
	"linux": {
		{"The 6.20 kernel lands with the Rust GPU driver merged", "lwn.net", "News"},
		{"I run my whole desktop from a 12MB static Go binary", "self.linux", ""},
		{"systemd vs. a 40-line shell script: a fair comparison", "blog.example.com", ""},
		{"Wayland is finally good, and here's the proof", "self.linux", ""},
	},
	"macos": {
		{"Reverse-engineering WKWebView's custom URL scheme handler", "developer.apple.com", "TIL"},
		{"A pure-Go .app bundle with no frameworks to sign", "github.com", "Show"},
		{"Sequoia broke my launchd job and here's the fix", "self.macos", ""},
		{"Menu bar apps in 2026: SwiftUI vs. AppKit vs. Go", "blog.example.com", ""},
	},
	"science": {
		{"New telescope survey doubles the count of known exoplanets", "nature.com", "News"},
		{"Room-temperature superconductor claim retracted, again", "science.org", ""},
		{"How mRNA design tools quietly got 100x faster", "self.science", ""},
		{"A readable explainer on the latest fusion net-gain result", "blog.example.com", "Article"},
	},
}

type demoItem struct {
	title, domain, flair string
}

func (demoFetcher) postsFor(name string) []reddit.Post {
	items, ok := demoContent[name]
	if !ok {
		// Unknown subreddit: synthesize a small themed set so switching still
		// shows distinct, name-specific content.
		items = []demoItem{
			{fmt.Sprintf("Welcome to r/%s — top post of the week", name), "self." + name, ""},
			{fmt.Sprintf("What everyone in r/%s is talking about today", name), "blog.example.com", ""},
			{fmt.Sprintf("A beginner's guide to r/%s", name), "self." + name, "Guide"},
		}
	}
	sub := name
	if sub == "" {
		sub = "popular"
	}
	posts := make([]reddit.Post, len(items))
	for i, it := range items {
		posts[i] = reddit.Post{
			ID:          fmt.Sprintf("%s%d", sub, i),
			Title:       it.title,
			Author:      fmt.Sprintf("user_%s_%d", sub, i+1),
			Subreddit:   sub,
			Domain:      it.domain,
			Score:       (len(items)-i)*613 + i*97,
			NumComments: (i+1)*37 + 3,
			Permalink:   fmt.Sprintf("/r/%s/comments/%s%d/demo/", sub, sub, i),
			Flair:       it.flair,
		}
	}
	return posts
}

func (d demoFetcher) Subreddit(_ context.Context, name string, _ reddit.Sort, _ reddit.ListingOptions) (*reddit.Page, error) {
	return &reddit.Page{Posts: d.postsFor(name)}, nil
}

func (d demoFetcher) Frontpage(_ context.Context, _ reddit.Sort, _ reddit.ListingOptions) (*reddit.Page, error) {
	return &reddit.Page{Posts: d.postsFor("")}, nil
}

func (d demoFetcher) Comments(_ context.Context, _, id string, _ reddit.ListingOptions) (*reddit.PostWithComments, error) {
	return &reddit.PostWithComments{
		Post:     reddit.Post{ID: id, Title: "Demo post", Author: "demo"},
		Comments: []reddit.Comment{{ID: "c1", Author: "commenter", Body: "This is a demo comment tree.", Score: 12}},
	}, nil
}

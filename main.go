// Command reader is a native macOS app that browses Reddit with a go-widgets
// UI. It runs a local pure-Go HTTP server that (a) serves the WebAssembly
// front-end built from ./cmd/front and (b) proxies /api/* to Reddit through
// github.com/go-reddit/reddit, then opens a native WKWebView window (via
// purego, CGO=0) pointed at that server. Off macOS — or with -no-window — it
// prints the URL and opens the default browser instead.
//
//	reader -sr golang -sort top
//	READER_OAUTH_CLIENT_ID=… READER_OAUTH_CLIENT_SECRET=… reader
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/go-reddit/reader/internal/auth"
	"github.com/go-reddit/reader/internal/server"
	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-reddit/reader/internal/webview"
)

func main() {
	// Cocoa's run loop must own the main OS thread.
	runtime.LockOSThread()

	opts := optionsFromEnv(getenv)
	var demo, serveOnly, useHTTP bool
	flag.StringVar(&opts.subreddit, "sr", opts.subreddit, "subreddit to open (empty = front page)")
	flag.StringVar(&opts.sort, "sort", opts.sort, "sort: hot|new|top|rising|controversial|best")
	flag.BoolVar(&opts.noWindow, "no-window", false, "skip the native window; open the default browser instead")
	flag.BoolVar(&demo, "demo", false, "serve a built-in sample feed instead of hitting Reddit (no credentials needed)")
	flag.BoolVar(&serveOnly, "serve-only", false, "run over a loopback TCP port and print its URL; open neither a window nor a browser")
	flag.BoolVar(&useHTTP, "http", false, "use a loopback TCP server instead of the in-process URL-scheme transport")
	flag.Parse()

	var fetcher server.Fetcher = opts.newClient()
	if demo {
		fetcher = demoFetcher{}
	}
	srv := server.New(fetcher, server.Assets)
	if path, err := settings.DefaultPath(); err == nil {
		srv.SetSettings(settings.NewStore(path))
	} else {
		log.Printf("reader: settings disabled: %v", err)
	}

	// Touch-ID-gated OAuth login: on success the server's client is swapped
	// for an authenticated one.
	authSvc := auth.NewService(auth.NewVault(), func(c auth.Credentials) {
		srv.SetFetcher(opts.oauthClientFor(c.ClientID, c.ClientSecret))
	})
	srv.SetLogin(authSvc)

	// Default macOS transport: serve everything in-process over a private URL
	// scheme (no socket at all). -http, -serve-only, -no-window and browser
	// fallback all need a real loopback address, so they take the TCP path.
	if !useHTTP && !serveOnly && !opts.noWindow {
		cfg := webview.Config{
			Title:     "Reddit — go-widgets",
			URL:       feedURL("", opts), // path-only under the scheme origin
			Width:     900,
			Height:    660,
			Handler:   srv,
			Scheme:    "reader",
			MenuTitle: "R", // menu-bar (tray) status item
			OnLogin:   func() { _ = authSvc.Unlock() },
			OnLogout:  func() { _ = authSvc.Logout() },
		}
		if err := webview.Run(cfg); err == nil {
			return
		} else {
			log.Printf("reader: scheme transport unavailable (%v); falling back to loopback HTTP", err)
		}
	}

	// TCP loopback path. 127.0.0.1:0 => the OS picks a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("reader: listen: %v", err)
	}
	go func() {
		if err := http.Serve(ln, srv); err != nil {
			log.Printf("reader: server stopped: %v", err)
		}
	}()
	target := feedURL("http://"+ln.Addr().String(), opts)
	log.Printf("reader: serving %s", target)

	switch {
	case serveOnly:
		select {} // run headless; caller drives the URL (automation, embedding)
	case opts.noWindow:
		openBrowser(target)
		select {}
	default:
		if err := webview.Run(webview.Config{Title: "Reddit — go-widgets", URL: target, Width: 900, Height: 660}); err != nil {
			log.Printf("reader: native window unavailable (%v); opening browser", err)
			openBrowser(target)
			select {}
		}
	}
}

// startCommand launches a detached command. It is a package var so tests can
// exercise openBrowser's logic WITHOUT actually spawning a browser (a test
// that called the real thing would pop a window open on every `go test`).
var startCommand = func(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// openBrowser opens url in the platform's default browser. Best-effort: a
// failure is logged, not fatal (the URL is already printed).
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	if err := startCommand(cmd, append(args, url)...); err != nil {
		fmt.Printf("open %s in your browser\n", url)
	}
}

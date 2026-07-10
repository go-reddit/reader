# go-reddit/reader

A native **macOS** Reddit reader whose entire UI is drawn by
[go-widgets](https://github.com/go-widgets/toolkit) — compiled to WebAssembly,
blitted into a `<canvas>`, and hosted in a **WKWebView** that is opened from Go
with **zero cgo** (`CGO_ENABLED=0`).

<p align="center"><img src="docs/screenshot.png" alt="go-reddit reader" width="720"></p>

## What it is

```
┌─ "Reddit Reader.app"  (pure Go, CGO=0, self-contained) ───────────────┐
│                                                                        │
│   internal/ui        go-widgets toolkit renders the feed → RGBA buffer │
│   cmd/front  (wasm)  blits that buffer into a browser <canvas>         │
│   internal/server    serves the wasm + proxies /api → Reddit           │
│   internal/webview   opens a WKWebView via purego (no cgo)             │
│                                                                        │
│   Transport (default): a WKURLSchemeHandler serves every request       │
│   in-process — no TCP, no unix socket, no listening port at all.       │
│                                                                        │
│        fetch  reader://app/api/feed ──► go-reddit/reddit ──► reddit.com │
└────────────────────────────────────────────────────────────────────────┘
```

The layout mirrors the Reddit web UI: a **topbar** with sort tabs
(hot/new/top/rising) and the current feed, a **sidebar** of bookmarked
subreddits, and the scrollable **feed** of post cards. Clicking a sidebar feed
or a sort tab reloads; clicking a post opens it.

Three design choices worth calling out:

1. **go-widgets, not HTML.** The cards, score badges, sidebar and topbar are
   painted by the go-widgets painter into an RGBA buffer, exactly as in a
   native window — the WebView is just a surface. Text is anti-aliased
   TrueType (the embedded Go fonts) rendered into that same buffer at device
   resolution, so it stays crisp at any zoom or on a Retina display.
2. **CGO=0 everywhere.** The WKWebView, NSWindow and NSApplication are driven
   through the Objective-C runtime via
   [purego](https://github.com/ebitengine/purego). The shipped binary links
   only system libraries; AppKit/WebKit are `dlopen`ed at runtime.
3. **No socket.** Instead of a loopback HTTP server, the default transport
   registers a private `reader://` URL scheme and answers every request
   in-process from the same `http.Handler`. Nothing on the machine can reach
   the app's content, and no port is consumed. (A unix socket wouldn't help
   here — WKWebView can't dial one; the scheme handler is strictly better.)
   Pass `-http` to fall back to a `127.0.0.1` server (used off macOS and for
   automated testing).

## Try it

```sh
# Real Reddit (front page; clicking a sidebar feed loads that subreddit):
task run                       # == go run .
go run . -sr golang -sort top  # start on a specific feed

# Build a double-clickable app bundle:
task app                       # produces dist/Reddit Reader.app
open "dist/Reddit Reader.app"
```

**Reddit increasingly blocks anonymous `.json` access with a 403** (always from
datacenter IPs, sometimes from residential). If you see the 403 message in the
status bar, use OAuth — register an app at <https://www.reddit.com/prefs/apps>
and export its credentials; the reader then authenticates and works everywhere:

```sh
READER_OAUTH_CLIENT_ID=…  READER_OAUTH_CLIENT_SECRET=…  task run
```

Offline / no-credentials demo (built-in per-subreddit sample feeds — good for
trying the UI and the sidebar switching without a network):

```sh
task demo                      # == go run . -demo
```

### Flags & environment

| Flag | Env | Meaning |
|------|-----|---------|
| `-sr` | `READER_SUBREDDIT` | subreddit (empty = front page) |
| `-sort` | `READER_SORT` | `hot`/`new`/`top`/`rising`/`controversial`/`best` |
| `-demo` | | serve a built-in sample feed, no network |
| `-http` | | use a loopback TCP server instead of the URL-scheme transport |
| `-no-window` | | open the default browser instead of a native window |
| `-serve-only` | | run headless over TCP and print the URL |
| | `READER_OAUTH_CLIENT_ID` / `_SECRET` | app-only OAuth |
| | `READER_OAUTH_USERNAME` / `_PASSWORD` | script-grant OAuth |

### Keyboard & window

- **Drag the window edge** — the feed re-lays-out at the new size (cards
  widen/narrow, titles reflow), it does not stretch.
- **⌘ + / ⌘ -** — zoom the display in/out; **⌘ 0** resets. Zoom renders the
  go-widgets UI at a smaller/larger logical resolution (bigger text and cards,
  fewer at once), staying crisp rather than blurring.
- **Scroll** — browse the feed; **click a post** — open it.

## Build layout

- `internal/ui` — the go-widgets scene: layout, hit-testing, rendering. Pure,
  native-testable (100% coverage), no build tag.
- `cmd/front` — the `GOOS=js GOARCH=wasm` entry point (syscall/js glue).
- `internal/server` — static bundle + `/api` proxy (100% coverage).
- `internal/webview` — the purego WKWebView + `WKURLSchemeHandler` bridge.

The wasm bundle is a build artifact; `scripts/build-wasm.sh` (run by every
`task` target) produces `internal/server/assets/reader.wasm`.

## Testing notes

Everything that can run without a display is covered by native tests: the UI
scene renders to a byte buffer checked pixel-for-non-blank (and dumped to PNG
when `READER_PNG` is set), the server is driven through `httptest`, and the
Objective-C bridge is exercised against the **real** runtime (NSString round
trips, NSData, class registration, and the scheme-handler request/response path
via a mock `WKURLSchemeTask`). The window creation and Cocoa run loop
(`webview.Run`) are a native-integration boundary verified by launching the
built app; they are the only substantial code the unit tests can't reach on a
headless machine.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-reddit/reader authors.

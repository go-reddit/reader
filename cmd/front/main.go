// Command front is the WebAssembly front-end of the reader. It renders the
// go-widgets Reddit UI ([ui.Scene]) into a browser <canvas> and talks to the
// host proxy (/api/feed) over same-origin fetch, so no Reddit CORS or
// credentials ever reach the page. Build with:
//
//	GOOS=js GOARCH=wasm go build -o reader.wasm ./cmd/front
//
// The scene's layout, hit-testing and rendering live in internal/ui and are
// covered by native tests; this file is the thin syscall/js glue that a
// browser exercises end-to-end.
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"syscall/js"

	"github.com/go-reddit/reddit"
	"github.com/go-widgets/toolkit"
	"github.com/go-reddit/reader/internal/ui"
)

func main() {
	doc := js.Global().Get("document")
	canvas := doc.Call("getElementById", "screen")
	if !canvas.Truthy() {
		js.Global().Get("console").Call("error", "reader: no #screen canvas")
		return
	}

	scene := ui.NewScene()
	if prefersDark() {
		scene.SetTheme(toolkit.DefaultDark())
	}
	sr, sort := feedFromQuery()
	scene.SetFeed(sr, sort)

	ctx := canvas.Call("getContext", "2d")

	// The canvas fills the window and the buffer is rendered at DEVICE
	// resolution (CSS size × devicePixelRatio, displayed 1:1) so the
	// anti-aliased text stays crisp with no upscale blur. Zoom is a UI scale
	// factor (bigger fonts + metrics), combined with the device ratio.
	var (
		imageData js.Value
		buf       []byte
		zoom      = 1.0
	)
	resize := func() {
		dpr := js.Global().Get("devicePixelRatio").Float()
		if dpr <= 0 {
			dpr = 1
		}
		cw := viewportSize(canvas, "clientWidth", 900)
		ch := viewportSize(canvas, "clientHeight", 660)
		w := int(float64(cw)*dpr + 0.5)
		h := int(float64(ch)*dpr + 0.5)
		scene.Scale = zoom * dpr
		scene.Resize(w, h)
		w, h = scene.W, scene.H // Resize clamps to a minimum
		canvas.Set("width", w)
		canvas.Set("height", h)
		imageData = ctx.Call("createImageData", w, h)
		buf = make([]byte, w*h*4)
	}

	render := func() {
		scene.Draw(buf)
		js.CopyBytesToJS(imageData.Get("data"), buf)
		ctx.Call("putImageData", imageData, 0, 0)
	}

	resize()

	// Re-layout when the window (and thus the canvas) changes size.
	js.Global().Call("addEventListener", "resize", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		resize()
		render()
		return nil
	}))

	// Cmd +/- change the display zoom; Cmd 0 resets it. We intercept these so
	// they scale the go-widgets UI rather than the WebView's own magnification.
	setZoom := func(z float64) {
		if z != zoom {
			zoom = z
			resize()
			render()
		}
	}
	js.Global().Call("addEventListener", "keydown", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		ev := args[0]
		if !(ev.Get("metaKey").Bool() || ev.Get("ctrlKey").Bool()) {
			return nil
		}
		switch ev.Get("key").String() {
		case "+", "=": // Cmd+= is "zoom in" without Shift on US layouts
			ev.Call("preventDefault")
			setZoom(ui.StepZoom(zoom, +1))
		case "-", "_":
			ev.Call("preventDefault")
			setZoom(ui.StepZoom(zoom, -1))
		case "0":
			ev.Call("preventDefault")
			setZoom(1.0)
		}
		return nil
	}))

	// Click: open a post, or switch feed/sort (which reloads) via the sidebar
	// and topbar.
	canvas.Call("addEventListener", "click", js.FuncOf(func(_ js.Value, args []js.Value) any {
		x, y := eventXY(canvas, args)
		switch hit := scene.HitTest(x, y); hit.Kind {
		case ui.HitPost:
			if hit.Post.FullPermalink() != "" {
				js.Global().Call("open", hit.Post.FullPermalink(), "_blank")
			}
		case ui.HitFeed:
			scene.SetFeed(hit.Feed, scene.Sort)
			render()
			loadFeed(scene, scene.Subreddit, scene.Sort, render)
		case ui.HitSort:
			scene.SetFeed(scene.Subreddit, hit.Sort)
			render()
			loadFeed(scene, scene.Subreddit, scene.Sort, render)
		}
		return nil
	}))

	// Wheel: scroll the feed.
	canvas.Call("addEventListener", "wheel", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			args[0].Call("preventDefault")
			if scene.Scroll(int(args[0].Get("deltaY").Float())) {
				render()
			}
		}
		return nil
	}), map[string]any{"passive": false})

	render()
	loadFeed(scene, sr, sort, render)
	select {} // park so the callbacks stay live
}

// viewportSize reads the canvas element's current CSS pixel size (clientWidth
// /clientHeight, which track the window because the canvas fills it), falling
// back to def when the value is not yet available (0 before first layout).
func viewportSize(canvas js.Value, prop string, def int) int {
	if v := canvas.Get(prop).Int(); v > 0 {
		return v
	}
	return def
}

// prefersDark reports the OS/browser colour-scheme preference.
func prefersDark() bool {
	mql := js.Global().Call("matchMedia", "(prefers-color-scheme: dark)")
	return mql.Truthy() && mql.Get("matches").Bool()
}

// feedFromQuery reads ?sr= and ?sort= from the page URL.
func feedFromQuery() (sr, sort string) {
	search := js.Global().Get("location").Get("search").String()
	q, err := url.ParseQuery(trimLeadingQuestion(search))
	if err != nil {
		return "", "hot"
	}
	sort = q.Get("sort")
	if sort == "" {
		sort = "hot"
	}
	return q.Get("sr"), sort
}

func trimLeadingQuestion(s string) string {
	if len(s) > 0 && s[0] == '?' {
		return s[1:]
	}
	return s
}

// eventXY converts a mouse event's client coordinates to canvas-local pixels,
// accounting for CSS scaling of the canvas element.
func eventXY(canvas js.Value, args []js.Value) (int, int) {
	if len(args) == 0 {
		return -1, -1
	}
	ev := args[0]
	rect := canvas.Call("getBoundingClientRect")
	w := rect.Get("width").Float()
	h := rect.Get("height").Float()
	if w == 0 || h == 0 {
		return -1, -1
	}
	sx := float64(canvas.Get("width").Int()) / w
	sy := float64(canvas.Get("height").Int()) / h
	x := int((ev.Get("clientX").Float() - rect.Get("left").Float()) * sx)
	y := int((ev.Get("clientY").Float() - rect.Get("top").Float()) * sy)
	return x, y
}

// loadFeed fetches the current feed from the host proxy in a goroutine and
// repaints when it arrives (or shows the error in the status bar).
func loadFeed(scene *ui.Scene, sr, sort string, render func()) {
	scene.Status = "Loading " + scene.FeedName() + " …"
	render()
	go func() {
		q := url.Values{}
		if sr != "" {
			q.Set("sr", sr)
		}
		q.Set("sort", sort)
		q.Set("limit", strconv.Itoa(50))

		resp, err := http.Get("/api/feed?" + q.Encode())
		if err != nil {
			scene.Status = "network error: " + err.Error()
			render()
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			scene.Status = "reddit error: HTTP " + strconv.Itoa(resp.StatusCode)
			render()
			return
		}
		var page reddit.Page
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			scene.Status = "decode error: " + err.Error()
			render()
			return
		}
		scene.SetPosts(page.Posts)
		scene.Status = ""
		render()
	}()
}

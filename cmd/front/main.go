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
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall/js"
	"unicode/utf8"

	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-reddit/reader/internal/ui"
	"github.com/go-reddit/reddit"
)

func main() {
	doc := js.Global().Get("document")
	canvas := doc.Call("getElementById", "screen")
	if !canvas.Truthy() {
		js.Global().Get("console").Call("error", "reader: no #screen canvas")
		return
	}

	scene := ui.NewScene()
	osName := detectOS()
	querySR, querySort := feedFromQuery()
	if querySort != "" {
		scene.Sort = querySort
	}
	applyTheme := func() { scene.SetTheme(ui.ResolveTheme(scene.ThemeName, osName, prefersDark())) }
	applyTheme()

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

	// persist writes the current profiles/sort/theme back to the host.
	persist := func() {
		b, err := json.Marshal(scene.Settings())
		if err != nil {
			return
		}
		go func() {
			req, err := http.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(b))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}()
	}

	// openFeed selects a subreddit + reloads its posts.
	openFeed := func(sr string) {
		scene.SetFeed(sr, scene.Sort)
		render()
		loadFeed(scene, scene.Subreddit, scene.Sort, render)
	}

	// firstFeed is the initial subreddit: the URL's ?sr= if present, else the
	// first feed of the active profile.
	firstFeed := func() string {
		if querySR != "" {
			return querySR
		}
		if f := scene.ActiveFeeds(); len(f) > 0 {
			return f[0]
		}
		return ""
	}

	// submitLogin POSTs the typed credentials; the host prompts Touch ID and
	// stores them, then the client is reconfigured to OAuth and the feed
	// reloads. On failure the error shows under the form.
	submitLogin := func() {
		id, sec := scene.LoginCredentials()
		b, _ := json.Marshal(map[string]string{"client_id": id, "client_secret": sec})
		scene.SetLoginError("Authenticating with Touch ID…")
		render()
		go func() {
			resp, err := http.Post("/api/login", "application/json", bytes.NewReader(b))
			if err != nil {
				scene.SetLoginError("network error: " + err.Error())
				render()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				scene.SetLoginError("login failed (HTTP " + strconv.Itoa(resp.StatusCode) + ") — check credentials / Touch ID")
				render()
				return
			}
			scene.LoggedIn = true
			scene.Mode = ui.ModeFeed
			render()
			openFeed(firstFeed())
		}()
	}

	doLogout := func() {
		go func() {
			if resp, err := http.Post("/api/logout", "application/json", nil); err == nil {
				resp.Body.Close()
			}
			scene.LoggedIn = false
			render()
			openFeed(firstFeed())
		}()
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
		key := ev.Get("key").String()
		if ev.Get("metaKey").Bool() || ev.Get("ctrlKey").Bool() {
			switch key {
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
		}
		// Text entry for the in-canvas editors.
		switch scene.Mode {
		case ui.ModeSettings:
			switch {
			case key == "Enter":
				ev.Call("preventDefault")
				scene.AddInputFeed()
				persist()
				render()
			case key == "Backspace":
				ev.Call("preventDefault")
				scene.Backspace()
				render()
			case utf8.RuneCountInString(key) == 1:
				ev.Call("preventDefault")
				scene.TypeRune([]rune(key)[0])
				render()
			}
		case ui.ModeLogin:
			switch {
			case key == "Enter":
				ev.Call("preventDefault")
				submitLogin()
			case key == "Tab":
				ev.Call("preventDefault")
				scene.NextLoginField()
				render()
			case key == "Backspace":
				ev.Call("preventDefault")
				scene.Backspace()
				render()
			case utf8.RuneCountInString(key) == 1:
				ev.Call("preventDefault")
				scene.TypeRune([]rune(key)[0])
				render()
			}
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
			openFeed(hit.Feed)
		case ui.HitSort:
			scene.Sort = hit.Sort
			if scene.Mode == ui.ModeSettings {
				persist()
				render()
			} else {
				openFeed(scene.Subreddit)
			}
		case ui.HitProfile:
			scene.SetActive(hit.Profile)
			persist()
			openFeed(firstFeed())
		case ui.HitSettings:
			scene.OpenSettings()
			render()

		// --- settings editor ---
		case ui.HitCloseSettings:
			scene.CloseSettings()
			persist()
			render()
			openFeed(firstFeed())
		case ui.HitTheme:
			scene.SetThemeName(hit.Value)
			applyTheme()
			persist()
			render()
		case ui.HitSelectProfile:
			scene.SelectEdit(hit.Profile)
			render()
		case ui.HitRemoveFeed:
			scene.RemoveFeed(hit.Profile, hit.Value)
			persist()
			render()
		case ui.HitAddFeed:
			scene.AddInputFeed()
			persist()
			render()
		case ui.HitNewProfile:
			scene.NewProfile()
			persist()
			render()
		case ui.HitDeleteProfile:
			scene.DeleteProfile(hit.Profile)
			persist()
			render()

		// --- account / login ---
		case ui.HitOpenLogin:
			scene.OpenLogin()
			render()
		case ui.HitLogout:
			doLogout()
		case ui.HitLoginField:
			scene.FocusLoginField(hit.Profile)
			render()
		case ui.HitLoginSubmit:
			submitLogin()
		case ui.HitLoginCancel:
			scene.Mode = ui.ModeFeed
			render()
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
	// Load persisted settings, then open the initial feed.
	go func() {
		if resp, err := http.Get("/api/settings"); err == nil {
			var st settings.Settings
			if resp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp.Body).Decode(&st)
			}
			resp.Body.Close()
			if len(st.Profiles) > 0 {
				scene.SetProfiles(st.Profiles, st.Active)
				scene.Sort = st.Sort
				scene.ThemeName = st.Theme
			}
		}
		if querySort != "" { // URL overrides the stored sort
			scene.Sort = querySort
		}
		// Reflect the current auth state in the sidebar.
		if resp, err := http.Get("/api/login/status"); err == nil {
			var st struct {
				LoggedIn bool `json:"logged_in"`
			}
			json.NewDecoder(resp.Body).Decode(&st)
			resp.Body.Close()
			scene.LoggedIn = st.LoggedIn
		}
		applyTheme()
		openFeed(firstFeed())
	}()
	select {} // park so the callbacks stay live
}

// detectOS derives the OS token for [ui.ResolveTheme] from the browser.
func detectOS() string {
	nav := js.Global().Get("navigator")
	p := ""
	if uad := nav.Get("userAgentData"); uad.Truthy() {
		p = uad.Get("platform").String()
	}
	if p == "" {
		p = nav.Get("platform").String()
	}
	if p == "" {
		p = nav.Get("userAgent").String()
	}
	switch ls := strings.ToLower(p); {
	case strings.Contains(ls, "mac"):
		return ui.OSMac
	case strings.Contains(ls, "win"):
		return ui.OSWindows
	case strings.Contains(ls, "linux"), strings.Contains(ls, "x11"):
		return ui.OSLinux
	default:
		return ""
	}
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
			msg := "reddit error: HTTP " + strconv.Itoa(resp.StatusCode)
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
				msg += " — reddit blocked anonymous access; set READER_OAUTH_CLIENT_ID/SECRET (or try -demo)"
			}
			scene.Status = msg
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

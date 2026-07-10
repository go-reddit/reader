// Reddit browser engine: a hidden WKWebView loaded on the reddit.com origin
// that performs same-origin fetch("/r/…/….json") calls and posts the results
// back to Go via a WKScriptMessageHandler. Because the requests run inside a
// real browser on reddit.com (real cookies, real headers), they are NOT hit by
// the anti-bot 403 that blocks non-browser clients and datacenter IPs — which
// is the only way to read Reddit now that self-serve API keys are gone.
//
//go:build darwin

package webview

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego/objc"
)

var (
	selUserContentController   = objc.RegisterName("userContentController")
	selAddScriptMessageHandler = objc.RegisterName("addScriptMessageHandler:name:")
	selAddUserScript           = objc.RegisterName("addUserScript:")
	selInitUserScript          = objc.RegisterName("initWithSource:injectionTime:forMainFrameOnly:")
	selBody                    = objc.RegisterName("body")
	selEvaluateJS              = objc.RegisterName("evaluateJavaScript:completionHandler:")
)

// WKUserScriptInjectionTimeAtDocumentEnd.
const injectionTimeAtDocumentEnd = 1

type redditEngine struct {
	webview objc.ID

	mu      sync.Mutex
	pending map[string]chan engineResult
	ready   chan struct{}
	readyed bool
	nextID  int
}

type engineResult struct {
	body string
	err  string
}

var engine *redditEngine

// installRedditEngine creates the hidden reddit.com web view + message bridge.
// Called on the main thread from Run.
func installRedditEngine() error {
	if err := loadFrameworks(); err != nil {
		return err
	}
	e := &redditEngine{pending: map[string]chan engineResult{}, ready: make(chan struct{})}

	cfg := objc.ID(objc.GetClass("WKWebViewConfiguration")).Send(selAlloc).Send(selInit)
	ucc := cfg.Send(selUserContentController)

	handler, err := newBridgeHandler()
	if err != nil {
		return err
	}
	ucc.Send(selAddScriptMessageHandler, handler, nsString("reddit"))

	// A user script fires once the reddit.com DOM is ready, so fetches wait
	// until the origin is actually live.
	src := nsString(`window.webkit.messageHandlers.reddit.postMessage(JSON.stringify({ready:true}));`)
	us := objc.ID(objc.GetClass("WKUserScript")).Send(selAlloc).
		Send(selInitUserScript, src, injectionTimeAtDocumentEnd, true)
	ucc.Send(selAddUserScript, us)

	rect := cgRect{X: 0, Y: 0, W: 1, H: 1}
	wv := objc.ID(objc.GetClass("WKWebView")).Send(selAlloc).Send(selInitWithFrameConfiguration, rect, cfg)
	wv.Send(selRetain)
	nsurl := objc.ID(objc.GetClass("NSURL")).Send(selURLWithString, nsString("https://www.reddit.com/"))
	wv.Send(selLoadRequest, objc.ID(objc.GetClass("NSURLRequest")).Send(selRequestWithURL, nsurl))
	e.webview = wv

	engine = e
	return nil
}

// markReady is called by the bridge when the reddit page signals readiness.
func (e *redditEngine) markReady() {
	e.mu.Lock()
	if !e.readyed {
		e.readyed = true
		close(e.ready)
	}
	e.mu.Unlock()
}

// deliver routes a bridge message to the waiting fetch.
func (e *redditEngine) deliver(id string, r engineResult) {
	e.mu.Lock()
	ch := e.pending[id]
	delete(e.pending, id)
	e.mu.Unlock()
	if ch != nil {
		ch <- r
	}
}

// RedditFetch runs a same-origin fetch of pathAndQuery (e.g.
// "/r/golang/hot.json?limit=50") inside the reddit.com web view and returns the
// response body. Blocks until the engine is ready (or times out).
func RedditFetch(pathAndQuery string) (string, error) {
	if engine == nil {
		return "", errors.New("reddit engine not initialised")
	}
	// Wait for reddit.com to be live.
	select {
	case <-engine.ready:
	case <-time.After(20 * time.Second):
		return "", errors.New("reddit engine not ready (page did not load)")
	}

	engine.mu.Lock()
	engine.nextID++
	id := fmt.Sprintf("r%d", engine.nextID)
	ch := make(chan engineResult, 1)
	engine.pending[id] = ch
	engine.mu.Unlock()

	js := fetchJS(id, pathAndQuery)
	dispatchMain(func() {
		engine.webview.Send(selEvaluateJS, nsString(js), objc.ID(0))
	})

	select {
	case r := <-ch:
		if r.err != "" {
			return "", errors.New("reddit fetch: " + r.err)
		}
		return r.body, nil
	case <-time.After(25 * time.Second):
		engine.mu.Lock()
		delete(engine.pending, id)
		engine.mu.Unlock()
		return "", errors.New("reddit fetch timed out")
	}
}

// fetchJS builds the JavaScript that fetches the listing, keeps only "t3"
// (link) children, and posts {id, posts, after} back as a JSON string.
func fetchJS(id, path string) string {
	// JSON-encode the path and id as JS string literals (they are simple).
	pathLit := jsString(path)
	idLit := jsString(id)
	return `(function(){` +
		`fetch(` + pathLit + `,{credentials:'include',headers:{'Accept':'application/json'}})` +
		`.then(function(r){return r.json();})` +
		`.then(function(j){var p=[];if(j&&j.data&&j.data.children){p=j.data.children.filter(function(c){return c.kind==='t3';}).map(function(c){return c.data;});}` +
		`window.webkit.messageHandlers.reddit.postMessage(JSON.stringify({id:` + idLit + `,posts:p,after:(j.data&&j.data.after)||''}));})` +
		`.catch(function(e){window.webkit.messageHandlers.reddit.postMessage(JSON.stringify({id:` + idLit + `,err:String(e)}));});` +
		`})();`
}

// jsString renders s as a JS string literal.
func jsString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return "'" + s + "'"
}

// --- WKScriptMessageHandler bridge -----------------------------------------

var (
	bridgeOnce  sync.Once
	bridgeClass objc.Class
	bridgeErr   error
)

// newBridgeHandler returns an instance of a class conforming to
// WKScriptMessageHandler whose IMP routes messages to the engine.
func newBridgeHandler() (objc.ID, error) {
	bridgeOnce.Do(func() {
		proto := objc.GetProtocol("WKScriptMessageHandler")
		bridgeClass, bridgeErr = objc.RegisterClass(
			"GoRedditBridge", objc.GetClass("NSObject"),
			[]*objc.Protocol{proto}, nil,
			[]objc.MethodDef{
				{Cmd: objc.RegisterName("userContentController:didReceiveScriptMessage:"), Fn: bridgeDidReceive},
			})
	})
	if bridgeErr != nil {
		return 0, bridgeErr
	}
	inst := objc.ID(bridgeClass).Send(selAlloc).Send(selInit)
	inst.Send(selRetain)
	return inst, nil
}

// bridgeDidReceive is the IMP for -userContentController:didReceiveScriptMessage:.
func bridgeDidReceive(_ objc.ID, _ objc.SEL, _ objc.ID, message objc.ID) {
	if engine == nil {
		return
	}
	body := goString(message.Send(selBody))
	var m struct {
		ID    string `json:"id"`
		Ready bool   `json:"ready"`
		Err   string `json:"err"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return
	}
	if m.Ready {
		engine.markReady()
		return
	}
	if m.ID != "" {
		engine.deliver(m.ID, engineResult{body: body, err: m.Err})
	}
}

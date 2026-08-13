// Custom-URL-scheme transport for the WKWebView.
//
// Instead of running a TCP server and pointing the webview at
// http://127.0.0.1:PORT, this registers a WKURLSchemeHandler for a private
// scheme (e.g. "reader://") and serves every request in-process from a Go
// http.Handler. The upshot: no listening socket of any kind — not TCP, not a
// unix socket (which WKWebView could not dial anyway) — so nothing on the
// machine can reach the app's content, and no port is consumed.
//
// Requests arrive on the main thread inside -startURLSchemeTask:; we run the
// handler synchronously and reply immediately. Handling is cheap for the
// embedded assets; the one potentially slow path is the Reddit proxy, whose
// client carries its own timeout.
//
//go:build darwin

package webview

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"unsafe"

	objc "github.com/go-macos/objc"
)

var (
	selRequest            = objc.RegisterName("request")
	selURL                = objc.RegisterName("URL")
	selAbsoluteString     = objc.RegisterName("absoluteString")
	selHTTPMethod         = objc.RegisterName("HTTPMethod")
	selHTTPBody           = objc.RegisterName("HTTPBody")
	selDataLength         = objc.RegisterName("length")
	selDataGetBytes       = objc.RegisterName("getBytes:length:")
	selDataWithBytesLen   = objc.RegisterName("dataWithBytes:length:")
	selDictWithObjForKey  = objc.RegisterName("dictionaryWithObject:forKey:")
	selInitHTTPResponse   = objc.RegisterName("initWithURL:statusCode:HTTPVersion:headerFields:")
	selDidReceiveResponse = objc.RegisterName("didReceiveResponse:")
	selDidReceiveData     = objc.RegisterName("didReceiveData:")
	selDidFinish          = objc.RegisterName("didFinish")
	selSetURLSchemeFor    = objc.RegisterName("setURLSchemeHandler:forURLScheme:")
)

// handlerRegistry maps a scheme-handler instance (its objc pointer) to the Go
// http.Handler that serves it. A registry avoids stashing a Go pointer inside
// an Objective-C ivar (which the GC could not see).
var (
	handlerMu       sync.RWMutex
	handlerByObj    = map[uintptr]http.Handler{}
	cancelledTasks  = map[uintptr]bool{}
	schemeClassOnce sync.Once
	schemeClass     objc.Class
	schemeClassErr  error
)

// registerSchemeClass defines the Objective-C class implementing
// WKURLSchemeHandler exactly once. The class must FORMALLY conform to the
// WKURLSchemeHandler protocol (not merely implement its methods):
// -[WKWebViewConfiguration setURLSchemeHandler:forURLScheme:] validates
// conformsToProtocol:, so the protocol is declared via
// RegisterClassWithProtocols.
func registerSchemeClass() (objc.Class, error) {
	schemeClassOnce.Do(func() {
		schemeClass, schemeClassErr = objc.RegisterClassWithProtocols(
			"GoRedditSchemeHandler",
			objc.GetClass("NSObject"),
			[]*objc.Protocol{objc.GetProtocol("WKURLSchemeHandler")},
			[]objc.MethodDef{
				{Cmd: objc.RegisterName("webView:startURLSchemeTask:"), Fn: startURLSchemeTask},
				{Cmd: objc.RegisterName("webView:stopURLSchemeTask:"), Fn: stopURLSchemeTask},
			},
		)
	})
	return schemeClass, schemeClassErr
}

// newSchemeHandler builds an instance of the scheme-handler class bound to h.
func newSchemeHandler(h http.Handler) (objc.ID, error) {
	cls, err := registerSchemeClass()
	if err != nil {
		return 0, err
	}
	inst := objc.ID(cls).Send(selAlloc).Send(selInit)
	inst.Send(selRetain)
	handlerMu.Lock()
	handlerByObj[uintptr(inst)] = h
	handlerMu.Unlock()
	return inst, nil
}

// goRun runs the request handler off the calling thread; dispatchMain runs the
// response back on the main thread. Both are package vars so the mock-task test
// can make them synchronous. In production the handler must NOT run on the
// thread WebKit called us on (a blocking Reddit fetch would freeze the UI and
// WebKit would tear the task down mid-flight), and the WKURLSchemeTask methods
// MUST be messaged on the main thread. The main-thread hop is the shared
// github.com/go-macos/objc bridge's DispatchMain (libdispatch dispatch_async
// onto the main queue), so this package no longer dlopen's libSystem itself.
var (
	goRun        = func(fn func()) { go fn() }
	dispatchMain = objc.DispatchMain
)

// startURLSchemeTask is the IMP for -webView:startURLSchemeTask:. It reads the
// request on the calling (main) thread, serves it on a background goroutine so
// a slow upstream never blocks the UI, then replies on the main thread.
func startURLSchemeTask(self objc.ID, _ objc.SEL, _ objc.ID, task objc.ID) {
	handlerMu.RLock()
	h := handlerByObj[uintptr(self)]
	handlerMu.RUnlock()
	if h == nil {
		return
	}

	// Read everything WebKit-side up front (must be on this thread).
	req := task.Send(selRequest)
	nsurl := req.Send(selURL)
	rawURL := goString(nsurl.Send(selAbsoluteString))
	method := goString(req.Send(selHTTPMethod))
	if method == "" {
		method = http.MethodGet
	}
	body := nsDataBytes(req.Send(selHTTPBody))
	target := schemePathAndQuery(rawURL)

	goRun(func() {
		code, ct, data := serveSchemeRequest(h, method, target, body)
		dispatchMain(func() {
			if taskCancelled(task) {
				clearTask(task)
				return
			}
			respondToTask(task, nsurl, code, ct, data)
			clearTask(task)
		})
	})
}

// serveSchemeRequest runs the http.Handler against the mapped request and
// returns the recorded status, content type and body. Pure — no Objective-C —
// so it is unit-testable without WebKit.
func serveSchemeRequest(h http.Handler, method, target string, body []byte) (int, string, []byte) {
	rec := httptest.NewRecorder()
	hr, err := http.NewRequest(method, "http://app"+target, bytes.NewReader(body))
	if err != nil {
		return http.StatusBadRequest, "text/plain", []byte(err.Error())
	}
	h.ServeHTTP(rec, hr)
	return rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes()
}

// nsDataBytes copies an NSData's bytes into a Go slice using -getBytes:length:
// (a live Go pointer, so no uintptr→Pointer deref). Returns nil for an empty
// or nil NSData.
func nsDataBytes(d objc.ID) []byte {
	if d == 0 {
		return nil
	}
	n := int(d.Send(selDataLength))
	if n <= 0 {
		return nil
	}
	out := make([]byte, n)
	d.Send(selDataGetBytes, unsafe.Pointer(&out[0]), n)
	return out
}

// clearTask forgets a task's cancellation bookkeeping.
func clearTask(task objc.ID) {
	handlerMu.Lock()
	delete(cancelledTasks, uintptr(task))
	handlerMu.Unlock()
}

// stopURLSchemeTask is the IMP for -webView:stopURLSchemeTask:. WebKit calls
// it when it no longer wants a task's data; we mark it cancelled so a late
// completion never messages a dead task (which would raise).
func stopURLSchemeTask(_ objc.ID, _ objc.SEL, _ objc.ID, task objc.ID) {
	handlerMu.Lock()
	cancelledTasks[uintptr(task)] = true
	handlerMu.Unlock()
}

func taskCancelled(task objc.ID) bool {
	handlerMu.RLock()
	c := cancelledTasks[uintptr(task)]
	handlerMu.RUnlock()
	return c
}

// respondToTask replies to a WKURLSchemeTask with an HTTP status, content type
// and body.
func respondToTask(task, nsurl objc.ID, status int, contentType string, body []byte) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	headers := objc.ID(objc.GetClass("NSDictionary")).Send(selDictWithObjForKey,
		nsString(contentType), nsString("Content-Type"))
	resp := objc.ID(objc.GetClass("NSHTTPURLResponse")).Send(selAlloc).Send(
		selInitHTTPResponse, nsurl, status, nsString("HTTP/1.1"), headers)
	task.Send(selDidReceiveResponse, resp)
	task.Send(selDidReceiveData, nsData(body))
	task.Send(selDidFinish)
}

// nsData builds an NSData copy of b. An empty slice still yields a valid
// (zero-length) NSData.
func nsData(b []byte) objc.ID {
	var ptr unsafe.Pointer
	if len(b) > 0 {
		ptr = unsafe.Pointer(&b[0])
	} else {
		ptr = unsafe.Pointer(&([]byte{0})[0])
	}
	d := objc.ID(objc.GetClass("NSData")).Send(selDataWithBytesLen, uintptr(ptr), len(b))
	// b must stay alive until dataWithBytes:length: has copied it.
	runtime.KeepAlive(b)
	return d
}

// goString reads a Go string from an NSString via the shared objc bridge.
func goString(nsstr objc.ID) string {
	return objc.GoString(nsstr)
}

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
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

var (
	selRequest            = objc.RegisterName("request")
	selURL                = objc.RegisterName("URL")
	selAbsoluteString     = objc.RegisterName("absoluteString")
	selHTTPMethod         = objc.RegisterName("HTTPMethod")
	selLengthOfBytes      = objc.RegisterName("lengthOfBytesUsingEncoding:")
	selGetCString         = objc.RegisterName("getCString:maxLength:encoding:")
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
// WKURLSchemeHandler exactly once.
func registerSchemeClass() (objc.Class, error) {
	schemeClassOnce.Do(func() {
		proto := objc.GetProtocol("WKURLSchemeHandler")
		schemeClass, schemeClassErr = objc.RegisterClass(
			"GoRedditSchemeHandler",
			objc.GetClass("NSObject"),
			[]*objc.Protocol{proto},
			nil,
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

// startURLSchemeTask is the IMP for -webView:startURLSchemeTask:. It maps the
// custom-scheme request onto the Go handler and streams the recorded response
// back to WebKit.
func startURLSchemeTask(self objc.ID, _ objc.SEL, _ objc.ID, task objc.ID) {
	handlerMu.RLock()
	h := handlerByObj[uintptr(self)]
	handlerMu.RUnlock()
	if h == nil {
		return
	}

	req := task.Send(selRequest)
	nsurl := req.Send(selURL)
	rawURL := goString(nsurl.Send(selAbsoluteString))
	method := goString(req.Send(selHTTPMethod))
	if method == "" {
		method = http.MethodGet
	}

	// reader://app/api/feed?x=y  ->  /api/feed?x=y  (path+query as seen by
	// the http.Handler). net/url keeps the query on RequestURI.
	target := schemePathAndQuery(rawURL)

	rec := httptest.NewRecorder()
	hr, err := http.NewRequest(method, "http://app"+target, nil)
	if err != nil {
		return
	}
	h.ServeHTTP(rec, hr)

	if taskCancelled(task) {
		return
	}
	body := rec.Body.Bytes()
	respondToTask(task, nsurl, rec.Code, rec.Header().Get("Content-Type"), body)
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

// nsUTF8Encoding is NSUTF8StringEncoding.
const nsUTF8Encoding = 4

// goString reads a Go string from an NSString by copying its UTF-8 bytes into
// a Go-owned buffer with -getCString:maxLength:encoding:. Filling a Go slice
// (rather than dereferencing the -UTF8String raw pointer) keeps the code free
// of any uintptr→Pointer address arithmetic — the buffer pointer handed to
// ObjC is a live Go pointer the collector tracks.
func goString(nsstr objc.ID) string {
	if nsstr == 0 {
		return ""
	}
	n := int(nsstr.Send(selLengthOfBytes, nsUTF8Encoding))
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n+1) // +1 for the NUL getCString writes
	ok := nsstr.Send(selGetCString, unsafe.Pointer(&buf[0]), len(buf), nsUTF8Encoding)
	if ok == 0 {
		return ""
	}
	return string(buf[:n])
}

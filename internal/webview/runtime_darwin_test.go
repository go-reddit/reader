package webview

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ebitengine/purego/objc"
)

// These tests exercise the objc bridge against the REAL Objective-C runtime
// (available in any darwin process) without touching NSWindow/NSApplication/
// WKWebView, which require the main thread and a window server. The window +
// run-loop path is verified by launching the built app.

func TestLoadFrameworksIdempotent(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatalf("loadFrameworks: %v", err)
	}
	if err := loadFrameworks(); err != nil { // second call is a no-op
		t.Fatalf("second loadFrameworks: %v", err)
	}
}

func TestNSStringGoStringRoundTrip(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "hello", "r/golang · hot", "/api/feed?sr=golang&t=week"} {
		if got := goString(nsString(s)); got != s {
			t.Errorf("round trip %q -> %q", s, got)
		}
	}
	// A nil NSString yields an empty Go string.
	if got := goString(0); got != "" {
		t.Errorf("goString(nil) = %q", got)
	}
}

func TestNSDataLength(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	selLength := objc.RegisterName("length")
	for _, b := range [][]byte{nil, []byte("x"), []byte("hello world")} {
		d := nsData(b)
		if d == 0 {
			t.Fatalf("nsData(%q) = nil", b)
		}
		if n := int(d.Send(selLength)); n != len(b) {
			t.Errorf("NSData length = %d, want %d", n, len(b))
		}
	}
}

func TestRegisterSchemeClassOnce(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	c1, err := registerSchemeClass()
	if err != nil {
		t.Fatalf("registerSchemeClass: %v", err)
	}
	if c1 == 0 {
		t.Fatal("nil class")
	}
	c2, err := registerSchemeClass() // cached; must not re-allocate
	if err != nil || c2 != c1 {
		t.Fatalf("second call: class=%v err=%v", c2, err)
	}
}

func TestNewSchemeHandlerBindsHandler(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	inst, err := newSchemeHandler(h)
	if err != nil {
		t.Fatalf("newSchemeHandler: %v", err)
	}
	if inst == 0 {
		t.Fatal("nil instance")
	}
	handlerMu.RLock()
	bound := handlerByObj[uintptr(inst)]
	handlerMu.RUnlock()
	if bound == nil {
		t.Error("handler not registered for instance")
	}
}

// TestRespondToTaskBuildsResponse drives respondToTask's response/data
// construction against the real runtime using a stand-in object for the task.
// We can't synthesise a real WKURLSchemeTask, but we can confirm the response
// and data objects it builds are well-formed by inspecting them before they'd
// be handed to the task — so we build them the same way here.
func TestResponseObjectsWellFormed(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	nsurl := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("URLWithString:"), nsString("reader://app/x"))
	headers := objc.ID(objc.GetClass("NSDictionary")).Send(selDictWithObjForKey,
		nsString("application/json"), nsString("Content-Type"))
	resp := objc.ID(objc.GetClass("NSHTTPURLResponse")).Send(selAlloc).Send(
		selInitHTTPResponse, nsurl, 200, nsString("HTTP/1.1"), headers)
	if resp == 0 {
		t.Fatal("nil NSHTTPURLResponse")
	}
	if code := int(resp.Send(objc.RegisterName("statusCode"))); code != 200 {
		t.Errorf("statusCode = %d, want 200", code)
	}
}

func TestTaskCancellationTracking(t *testing.T) {
	// stopURLSchemeTask marks a task cancelled; taskCancelled reflects it.
	var fake objc.ID = 0xDEAD
	if taskCancelled(fake) {
		t.Fatal("task should start uncancelled")
	}
	stopURLSchemeTask(0, 0, 0, fake)
	if !taskCancelled(fake) {
		t.Error("task should be cancelled after stop")
	}
}

// --- mock WKURLSchemeTask ---------------------------------------------------
//
// We can't get WebKit to hand us a real task without a window, so we register
// a stand-in ObjC class exposing the three selectors startURLSchemeTask uses
// on the task (-request, -didReceiveResponse:, -didReceiveData:, -didFinish)
// and record what the handler sends it. This drives the real IMP body end to
// end through the live runtime.

var (
	mockOnce       sync.Once
	mockClass      objc.Class
	mockRequestObj objc.ID
	mockStatus     int
	mockDataLen    int
	mockFinished   bool
)

func mockTaskRequest(_ objc.ID, _ objc.SEL) objc.ID { return mockRequestObj }
func mockTaskDidReceiveResponse(_ objc.ID, _ objc.SEL, resp objc.ID) {
	mockStatus = int(resp.Send(objc.RegisterName("statusCode")))
}
func mockTaskDidReceiveData(_ objc.ID, _ objc.SEL, data objc.ID) {
	mockDataLen = int(data.Send(objc.RegisterName("length")))
}
func mockTaskDidFinish(_ objc.ID, _ objc.SEL) { mockFinished = true }

func newMockTask(t *testing.T, rawURL string) objc.ID {
	t.Helper()
	mockOnce.Do(func() {
		mockClass, _ = objc.RegisterClass("MockSchemeTask", objc.GetClass("NSObject"), nil, nil,
			[]objc.MethodDef{
				{Cmd: selRequest, Fn: mockTaskRequest},
				{Cmd: selDidReceiveResponse, Fn: mockTaskDidReceiveResponse},
				{Cmd: selDidReceiveData, Fn: mockTaskDidReceiveData},
				{Cmd: selDidFinish, Fn: mockTaskDidFinish},
			})
	})
	nsurl := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("URLWithString:"), nsString(rawURL))
	mockRequestObj = objc.ID(objc.GetClass("NSURLRequest")).Send(selRequestWithURL, nsurl)
	return objc.ID(mockClass).Send(selAlloc).Send(selInit)
}

func TestStartURLSchemeTaskFullPath(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	body := []byte(`[{"id":"x","title":"hi"}]`)
	var sawPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(body)
	})
	inst, err := newSchemeHandler(h)
	if err != nil {
		t.Fatal(err)
	}
	mockStatus, mockDataLen, mockFinished = 0, 0, false
	task := newMockTask(t, "reader://app/api/feed?sr=golang")

	// Invoke the real IMP directly with our mock task.
	startURLSchemeTask(inst, 0, 0, task)

	if sawPath != "/api/feed?sr=golang" {
		t.Errorf("handler saw %q", sawPath)
	}
	if mockStatus != 200 {
		t.Errorf("task status = %d, want 200", mockStatus)
	}
	if mockDataLen != len(body) {
		t.Errorf("task data length = %d, want %d", mockDataLen, len(body))
	}
	if !mockFinished {
		t.Error("task didFinish not called")
	}
}

func TestRespondToTaskDefaultsAndEmptyBody(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	task := newMockTask(t, "reader://app/x")
	nsurl := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("URLWithString:"), nsString("reader://app/x"))
	mockStatus, mockDataLen, mockFinished = 0, 0, false
	// Empty content type exercises the octet-stream default; nil body
	// exercises nsData's zero-length path.
	respondToTask(task, nsurl, 204, "", nil)
	if mockStatus != 204 || mockDataLen != 0 || !mockFinished {
		t.Errorf("respondToTask defaults: status=%d len=%d finished=%v", mockStatus, mockDataLen, mockFinished)
	}
}

func TestStartURLSchemeTaskUnknownHandler(t *testing.T) {
	// An instance never bound to a handler returns early (no panic).
	var stray objc.ID = 0xBEEF
	startURLSchemeTask(stray, 0, 0, 0)
}

func TestStartURLSchemeTaskCancelledSkipsResponse(t *testing.T) {
	if err := loadFrameworks(); err != nil {
		t.Fatal(err)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) })
	inst, _ := newSchemeHandler(h)
	task := newMockTask(t, "reader://app/")
	handlerMu.Lock()
	cancelledTasks[uintptr(task)] = true
	handlerMu.Unlock()
	mockFinished = false
	startURLSchemeTask(inst, 0, 0, task)
	if mockFinished {
		t.Error("cancelled task must not be completed")
	}
}

func TestSchemeHandlerServesThroughGoHandler(t *testing.T) {
	// End-to-end of the adaptation logic minus WebKit: the same path
	// startURLSchemeTask takes to reach the Go handler and record a response.
	called := ""
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	rec := httptest.NewRecorder()
	target := schemePathAndQuery("reader://app/api/feed?sr=golang")
	req, _ := http.NewRequest(http.MethodGet, "http://app"+target, nil)
	h.ServeHTTP(rec, req)
	if called != "/api/feed?sr=golang" {
		t.Errorf("handler saw %q", called)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Errorf("body = %q", rec.Body.String())
	}
}

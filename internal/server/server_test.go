package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-reddit/reddit"
)

// stubFetcher records the arguments it was called with and returns canned
// results, so the HTTP layer is tested without a live Reddit.
type stubFetcher struct {
	page    *reddit.Page
	comment *reddit.PostWithComments
	err     error

	gotName string
	gotSort reddit.Sort
	gotOpts reddit.ListingOptions
	gotID   string
	front   bool
}

func (s *stubFetcher) Subreddit(_ context.Context, name string, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error) {
	s.gotName, s.gotSort, s.gotOpts = name, sort, opts
	return s.page, s.err
}
func (s *stubFetcher) Frontpage(_ context.Context, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error) {
	s.front, s.gotSort, s.gotOpts = true, sort, opts
	return s.page, s.err
}
func (s *stubFetcher) Comments(_ context.Context, sr, id string, opts reddit.ListingOptions) (*reddit.PostWithComments, error) {
	s.gotName, s.gotID, s.gotOpts = sr, id, opts
	return s.comment, s.err
}

func testAssets() *fstest.MapFS {
	return &fstest.MapFS{
		"index.html": {Data: []byte("<html>reader</html>")},
	}
}

func TestServeIndex(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "reader") {
		t.Fatalf("index not served: %d %q", rr.Code, rr.Body.String())
	}
}

func TestHealthz(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != 200 || rr.Body.String() != "ok" {
		t.Fatalf("healthz = %d %q", rr.Code, rr.Body.String())
	}
}

func TestFeedSubreddit(t *testing.T) {
	stub := &stubFetcher{page: &reddit.Page{Posts: []reddit.Post{{Title: "hi"}}, After: "t3_x"}}
	srv := New(stub, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/feed?sr=golang&sort=top&limit=5&after=t3_a&t=week", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if stub.gotName != "golang" || stub.gotSort != reddit.SortTop {
		t.Errorf("passed name/sort = %q/%q", stub.gotName, stub.gotSort)
	}
	if stub.gotOpts.Limit != 5 || stub.gotOpts.After != "t3_a" || stub.gotOpts.Time != reddit.TimeWeek {
		t.Errorf("opts = %+v", stub.gotOpts)
	}
	var page reddit.Page
	if err := json.Unmarshal(rr.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Posts) != 1 || page.After != "t3_x" {
		t.Errorf("decoded page = %+v", page)
	}
}

func TestFeedFrontpageWhenNoSubreddit(t *testing.T) {
	stub := &stubFetcher{page: &reddit.Page{}}
	srv := New(stub, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/feed?limit=notanumber", nil))
	if rr.Code != 200 || !stub.front {
		t.Fatalf("expected frontpage call, code=%d front=%v", rr.Code, stub.front)
	}
	if stub.gotOpts.Limit != 0 { // bad limit ignored
		t.Errorf("bad limit should be ignored, got %d", stub.gotOpts.Limit)
	}
}

func TestFeedUpstreamError(t *testing.T) {
	stub := &stubFetcher{err: &reddit.APIError{StatusCode: http.StatusTooManyRequests, Status: "429"}}
	srv := New(stub, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/feed?sr=golang", nil))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "error") {
		t.Errorf("body = %q", rr.Body.String())
	}
}

func TestFeedNonAPIErrorIsBadGateway(t *testing.T) {
	stub := &stubFetcher{err: context.DeadlineExceeded}
	srv := New(stub, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/feed?sr=golang", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}

func TestFeedAPIErrorBelow400IsBadGateway(t *testing.T) {
	// An APIError with a non-HTTP status (e.g. 0 from an input-validation
	// failure) must not become a bogus HTTP code.
	stub := &stubFetcher{err: &reddit.APIError{StatusCode: 0, Status: "empty"}}
	srv := New(stub, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/feed?sr=golang", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}

func TestComments(t *testing.T) {
	stub := &stubFetcher{comment: &reddit.PostWithComments{
		Post:     reddit.Post{Title: "root"},
		Comments: []reddit.Comment{{Body: "hi"}},
	}}
	srv := New(stub, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/comments?sr=golang&id=abc123", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if stub.gotName != "golang" || stub.gotID != "abc123" {
		t.Errorf("passed sr/id = %q/%q", stub.gotName, stub.gotID)
	}
	var res reddit.PostWithComments
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Post.Title != "root" || len(res.Comments) != 1 {
		t.Errorf("decoded = %+v", res)
	}
}

func TestCommentsMissingID(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/comments", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestCommentsUpstreamError(t *testing.T) {
	stub := &stubFetcher{err: &reddit.APIError{StatusCode: http.StatusNotFound, Status: "404"}}
	srv := New(stub, testAssets())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/comments?id=abc", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestMustSubPanicsOnBadDir(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("mustSub should panic on an invalid sub-path")
		}
	}()
	mustSub(testAssets(), "../escape") // not a valid fs path
}

func TestAssetsEmbedded(t *testing.T) {
	// The real embedded bundle must carry the committed static files so a
	// built binary is self-contained.
	for _, name := range []string{"index.html", "wasm_exec.js"} {
		if _, err := Assets.Open(name); err != nil {
			t.Errorf("embedded asset %q missing: %v", name, err)
		}
	}
}

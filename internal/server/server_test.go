package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-reddit/reader/internal/auth"
	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-reddit/reddit"
)

// fakeStore is an in-memory SettingsStore.
type fakeStore struct {
	cur      settings.Settings
	loadErr  error
	saveErr  error
	saved    *settings.Settings
}

func (f *fakeStore) Load() (settings.Settings, error) { return f.cur, f.loadErr }
func (f *fakeStore) Save(s settings.Settings) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = &s
	return nil
}

func TestSettingsGet(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	srv.SetSettings(&fakeStore{cur: settings.Settings{Sort: "top", Profiles: []settings.Profile{{Name: "P"}}}})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	var got settings.Settings
	json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Sort != "top" {
		t.Errorf("got %+v", got)
	}
}

func TestSettingsPutSanitizes(t *testing.T) {
	fs := &fakeStore{}
	srv := New(&stubFetcher{}, testAssets())
	srv.SetSettings(fs)
	body := `{"profiles":[{"name":"P","feeds":["r/golang"," rust ","golang"]}],"sort":"new","theme":"dark"}`
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if fs.saved == nil || len(fs.saved.Profiles[0].Feeds) != 2 || fs.saved.Profiles[0].Feeds[0] != "golang" {
		t.Errorf("saved feeds not sanitised: %+v", fs.saved)
	}
}

func TestSettingsUnavailable(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets()) // no store set
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rr.Code)
	}
}

func TestSettingsBadJSON(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	srv.SetSettings(&fakeStore{})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader("{bad")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestSettingsMethodNotAllowed(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	srv.SetSettings(&fakeStore{})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/settings", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

func TestSettingsLoadError(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	srv.SetSettings(&fakeStore{loadErr: errors.New("disk gone")})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestSettingsSaveError(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	srv.SetSettings(&fakeStore{saveErr: errors.New("readonly")})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"sort":"hot"}`)))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rr.Code)
	}
}

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

// fakeLogin is an in-memory LoginService.
type fakeLogin struct {
	avail     bool
	loggedIn  bool
	loginErr  error
	unlockErr error
	logoutErr error
	gotCreds  auth.Credentials
}

func (f *fakeLogin) Available() bool { return f.avail }
func (f *fakeLogin) LoggedIn() bool  { return f.loggedIn }
func (f *fakeLogin) Login(c auth.Credentials) error {
	f.gotCreds = c
	if f.loginErr != nil {
		return f.loginErr
	}
	f.loggedIn = true
	return nil
}
func (f *fakeLogin) Unlock() error {
	if f.unlockErr != nil {
		return f.unlockErr
	}
	f.loggedIn = true
	return nil
}
func (f *fakeLogin) Logout() error {
	f.loggedIn = false
	return f.logoutErr
}

func TestLoginStatus(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	// No service: available=false, logged_in=false.
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/login/status", nil))
	var st struct {
		Available bool `json:"available"`
		LoggedIn  bool `json:"logged_in"`
	}
	json.Unmarshal(rr.Body.Bytes(), &st)
	if st.Available || st.LoggedIn {
		t.Errorf("no-service status = %+v", st)
	}
	srv.SetLogin(&fakeLogin{avail: true, loggedIn: true})
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/login/status", nil))
	json.Unmarshal(rr.Body.Bytes(), &st)
	if !st.Available || !st.LoggedIn {
		t.Errorf("service status = %+v", st)
	}
}

func TestLoginSuccessSwapsFetcher(t *testing.T) {
	fl := &fakeLogin{avail: true}
	srv := New(&stubFetcher{}, testAssets())
	srv.SetLogin(fl)
	rr := httptest.NewRecorder()
	body := `{"client_id":"id","client_secret":"sec"}`
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if fl.gotCreds.ClientID != "id" || !fl.loggedIn {
		t.Errorf("login not applied: %+v", fl)
	}
}

func TestLoginUnavailable(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	for _, path := range []string{"/api/login", "/api/logout"} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
		if rr.Code != http.StatusNotImplemented {
			t.Errorf("%s => %d, want 501", path, rr.Code)
		}
	}
	// unlock with no service.
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login/unlock", nil))
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("unlock no-service => %d", rr.Code)
	}
}

func TestLoginMethodAndBody(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	srv.SetLogin(&fakeLogin{avail: true})
	// GET not allowed.
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/login", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET login => %d", rr.Code)
	}
	// Bad JSON.
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("{bad")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad json => %d", rr.Code)
	}
}

func TestLoginError(t *testing.T) {
	srv := New(&stubFetcher{}, testAssets())
	srv.SetLogin(&fakeLogin{avail: true, loginErr: errors.New("touch id cancelled")})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"client_id":"a","client_secret":"b"}`)))
	if rr.Code != http.StatusBadGateway {
		t.Errorf("login error => %d", rr.Code)
	}
}

func TestUnlockAndLogout(t *testing.T) {
	fl := &fakeLogin{avail: true}
	srv := New(&stubFetcher{}, testAssets())
	srv.SetLogin(fl)
	// Unlock success.
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login/unlock", nil))
	if rr.Code != 200 || !fl.loggedIn {
		t.Errorf("unlock => %d loggedIn=%v", rr.Code, fl.loggedIn)
	}
	// Logout success.
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/logout", nil))
	if rr.Code != 200 || fl.loggedIn {
		t.Errorf("logout => %d loggedIn=%v", rr.Code, fl.loggedIn)
	}
	// Unlock error.
	fl2 := &fakeLogin{avail: true, unlockErr: errors.New("no creds")}
	srv2 := New(&stubFetcher{}, testAssets())
	srv2.SetLogin(fl2)
	rr = httptest.NewRecorder()
	srv2.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login/unlock", nil))
	if rr.Code != http.StatusBadGateway {
		t.Errorf("unlock error => %d", rr.Code)
	}
	// Logout error.
	fl3 := &fakeLogin{avail: true, logoutErr: errors.New("keychain")}
	srv3 := New(&stubFetcher{}, testAssets())
	srv3.SetLogin(fl3)
	rr = httptest.NewRecorder()
	srv3.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/logout", nil))
	if rr.Code != http.StatusBadGateway {
		t.Errorf("logout error => %d", rr.Code)
	}
}

func TestSetFetcherSwaps(t *testing.T) {
	first := &stubFetcher{page: &reddit.Page{After: "first"}}
	second := &stubFetcher{page: &reddit.Page{After: "second"}}
	srv := New(first, testAssets())
	srv.SetFetcher(second)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/feed?sr=golang", nil))
	var page reddit.Page
	json.Unmarshal(rr.Body.Bytes(), &page)
	if page.After != "second" {
		t.Errorf("SetFetcher not applied: After=%q", page.After)
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

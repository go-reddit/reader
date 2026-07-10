// Package server hosts the reader front-end: it serves the embedded wasm
// bundle and proxies the /api endpoints to Reddit through a go-reddit/reddit
// client. The proxy exists because the wasm UI runs in a WKWebView origin
// (http://127.0.0.1:PORT) from which reddit.com's ".json" API cannot be
// fetched directly — Reddit sends no permissive CORS headers. Routing through
// this same-origin proxy sidesteps CORS and keeps any OAuth credentials on
// the host side, never in the page.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-reddit/reader/internal/auth"
	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-reddit/reddit"
)

// Fetcher is the subset of *reddit.Client the server depends on. Narrowing to
// an interface lets tests inject a stub without a live network.
type Fetcher interface {
	Subreddit(ctx context.Context, name string, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error)
	Frontpage(ctx context.Context, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error)
	Comments(ctx context.Context, subreddit, id string, opts reddit.ListingOptions) (*reddit.PostWithComments, error)
}

// SettingsStore persists the reader's preferences (profiles/tabs, sort,
// theme). A nil store disables the settings endpoints.
type SettingsStore interface {
	Load() (settings.Settings, error)
	Save(settings.Settings) error
}

// LoginService is the subset of auth.Service the HTTP layer drives. A nil
// service disables the login endpoints.
type LoginService interface {
	Available() bool
	LoggedIn() bool
	Login(auth.Credentials) error
	Unlock() error
	Logout() error
}

// Server wires the Reddit client to an http.Handler.
type Server struct {
	mu       sync.RWMutex
	client   Fetcher // swappable at runtime (anonymous -> OAuth after login)
	assets   fs.FS
	settings SettingsStore
	login    LoginService
	mux      *http.ServeMux
}

// New builds a Server serving assets (the embedded web bundle) and proxying
// to client. The returned Server is an http.Handler.
func New(client Fetcher, assets fs.FS) *Server {
	s := &Server{client: client, assets: assets, mux: http.NewServeMux()}
	s.mux.Handle("/", http.FileServer(http.FS(assets)))
	s.mux.HandleFunc("/api/feed", s.handleFeed)
	s.mux.HandleFunc("/api/comments", s.handleComments)
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/login/status", s.handleLoginStatus)
	s.mux.HandleFunc("/api/login/unlock", s.handleUnlock)
	s.mux.HandleFunc("/api/logout", s.handleLogout)
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	return s
}

// SetSettings attaches a settings store, enabling the /api/settings endpoint.
func (s *Server) SetSettings(store SettingsStore) { s.settings = store }

// SetLogin attaches a login service, enabling the /api/login endpoints.
func (s *Server) SetLogin(l LoginService) { s.login = l }

// SetFetcher swaps the Reddit client at runtime (e.g. anonymous -> OAuth after
// a successful login). Safe for concurrent use.
func (s *Server) SetFetcher(f Fetcher) {
	s.mu.Lock()
	s.client = f
	s.mu.Unlock()
}

func (s *Server) getClient() Fetcher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// parseListingOptions reads the shared limit/after/time query parameters.
func parseListingOptions(r *http.Request) reddit.ListingOptions {
	opts := reddit.ListingOptions{After: r.URL.Query().Get("after")}
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		opts.Limit = l
	}
	if t := r.URL.Query().Get("t"); t != "" {
		opts.Time = reddit.TimeRange(t)
	}
	return opts
}

// handleFeed proxies GET /api/feed?sr=&sort=&limit=&after=&t= to a subreddit
// listing (or the front page when sr is empty), returning the decoded Page as
// JSON.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sort := reddit.Sort(q.Get("sort"))
	opts := parseListingOptions(r)

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	var (
		page *reddit.Page
		err  error
	)
	client := s.getClient()
	if sr := q.Get("sr"); sr != "" {
		page, err = client.Subreddit(ctx, sr, sort, opts)
	} else {
		page, err = client.Frontpage(ctx, sort, opts)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, page)
}

// handleComments proxies GET /api/comments?sr=&id= to a post's comment tree.
func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	res, err := s.getClient().Comments(ctx, q.Get("sr"), id, parseListingOptions(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, res)
}

// loginStatus is the JSON shape of /api/login/status.
type loginStatus struct {
	Available bool `json:"available"`
	LoggedIn  bool `json:"logged_in"`
}

func (s *Server) status() loginStatus {
	if s.login == nil {
		return loginStatus{}
	}
	return loginStatus{Available: s.login.Available(), LoggedIn: s.login.LoggedIn()}
}

// handleLoginStatus reports whether login is available and active.
func (s *Server) handleLoginStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.status())
}

// handleLogin accepts POSTed OAuth app credentials, runs the Touch-ID-gated
// vault save (via the service), and applies them. POST /api/login/unlock (no
// body) instead loads already-stored credentials behind Touch ID.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.login == nil {
		http.Error(w, `{"error":"login unavailable"}`, http.StatusNotImplemented)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var c auth.Credentials
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&c); err != nil {
		http.Error(w, `{"error":"invalid credentials json"}`, http.StatusBadRequest)
		return
	}
	if err := s.login.Login(c); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, s.status())
}

// handleUnlock loads already-stored credentials behind Touch ID.
func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if s.login == nil {
		http.Error(w, `{"error":"login unavailable"}`, http.StatusNotImplemented)
		return
	}
	if err := s.login.Unlock(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, s.status())
}

// handleLogout forgets the stored credentials.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.login == nil {
		http.Error(w, `{"error":"login unavailable"}`, http.StatusNotImplemented)
		return
	}
	if err := s.login.Logout(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, s.status())
}

// handleSettings serves GET (current settings) and PUT (replace settings) at
// /api/settings. Feeds in incoming profiles are sanitised before saving.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		http.Error(w, `{"error":"settings unavailable"}`, http.StatusNotImplemented)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cur, err := s.settings.Load()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, cur)
	case http.MethodPut, http.MethodPost:
		var in settings.Settings
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
			http.Error(w, `{"error":"invalid settings json"}`, http.StatusBadRequest)
			return
		}
		for i := range in.Profiles {
			in.Profiles[i].Feeds = settings.SanitizeFeeds(in.Profiles[i].Feeds)
		}
		if err := s.settings.Save(in); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, in)
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// writeJSON encodes v as JSON with the appropriate content type.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps a client error to an HTTP status. Reddit APIErrors preserve
// their upstream status (429/403/…); everything else is a 502 (bad upstream).
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var ae *reddit.APIError
	if errors.As(err, &ae) && ae.StatusCode >= 400 {
		status = ae.StatusCode
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

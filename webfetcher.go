package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-reddit/reader/internal/webview"
	"github.com/go-reddit/reddit"
)

// webFetcher implements server.Fetcher by fetching Reddit listings through the
// hidden reddit.com WKWebView (webview.RedditFetch). Because those fetches run
// inside a real browser on the reddit.com origin, they carry real cookies and
// headers and are not blocked by the anti-bot 403 — the only way to read Reddit
// now that self-serve API keys are gone. Comments open in the browser, so only
// listings are served here.
type webFetcher struct{}

// engineListing is the JSON the injected fetch script posts back.
type engineListing struct {
	Posts []reddit.Post `json:"posts"`
	After string        `json:"after"`
	Err   string        `json:"err"`
}

func (webFetcher) Subreddit(_ context.Context, name string, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "r/")
	if name == "" {
		return nil, errors.New("empty subreddit")
	}
	return webListing("/r/" + url.PathEscape(name) + "/" + sortOrHot(sort) + ".json" + listingQuery(opts))
}

func (webFetcher) Frontpage(_ context.Context, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error) {
	return webListing("/" + sortOrHot(sort) + ".json" + listingQuery(opts))
}

func (webFetcher) Comments(_ context.Context, _, id string, _ reddit.ListingOptions) (*reddit.PostWithComments, error) {
	// Posts open in the system browser; comments aren't rendered in-app.
	return &reddit.PostWithComments{Post: reddit.Post{ID: id}}, nil
}

// webListing runs one listing fetch through the engine and decodes it.
func webListing(path string) (*reddit.Page, error) {
	body, err := webview.RedditFetch(path)
	if err != nil {
		return nil, err
	}
	var r engineListing
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return nil, err
	}
	if r.Err != "" {
		return nil, errors.New("reddit: " + r.Err)
	}
	return &reddit.Page{Posts: r.Posts, After: r.After}, nil
}

func sortOrHot(s reddit.Sort) string {
	if s == "" {
		return "hot"
	}
	return string(s)
}

// listingQuery builds the ?limit=&after=&t= query for a listing path.
func listingQuery(opts reddit.ListingOptions) string {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.After != "" {
		q.Set("after", opts.After)
	}
	if opts.Time != "" {
		q.Set("t", string(opts.Time))
	}
	if enc := q.Encode(); enc != "" {
		return "?" + enc
	}
	return ""
}

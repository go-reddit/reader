package main

import (
	"context"

	"github.com/go-reddit/reader/internal/server"
	"github.com/go-reddit/reddit"
)

// chainFetcher tries each fetcher in order and returns the first success. It
// lets the portable uTLS client be primary while keeping the macOS WKWebView
// engine as a fallback: if uTLS is 403'd, the real-browser engine takes over.
type chainFetcher struct {
	fetchers []server.Fetcher
}

func (c chainFetcher) Subreddit(ctx context.Context, name string, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error) {
	var lastErr error
	for _, f := range c.fetchers {
		p, err := f.Subreddit(ctx, name, sort, opts)
		if err == nil {
			return p, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c chainFetcher) Frontpage(ctx context.Context, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error) {
	var lastErr error
	for _, f := range c.fetchers {
		p, err := f.Frontpage(ctx, sort, opts)
		if err == nil {
			return p, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c chainFetcher) Comments(ctx context.Context, subreddit, id string, opts reddit.ListingOptions) (*reddit.PostWithComments, error) {
	var lastErr error
	for _, f := range c.fetchers {
		p, err := f.Comments(ctx, subreddit, id, opts)
		if err == nil {
			return p, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

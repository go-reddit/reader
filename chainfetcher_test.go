package main

import (
	"context"
	"errors"
	"testing"

	"github.com/go-reddit/reader/internal/server"

	"github.com/go-reddit/reddit"
)

type stubF struct {
	page *reddit.Page
	err  error
	hits *int
}

func (s stubF) Subreddit(context.Context, string, reddit.Sort, reddit.ListingOptions) (*reddit.Page, error) {
	*s.hits++
	return s.page, s.err
}
func (s stubF) Frontpage(context.Context, reddit.Sort, reddit.ListingOptions) (*reddit.Page, error) {
	*s.hits++
	return s.page, s.err
}
func (s stubF) Comments(context.Context, string, string, reddit.ListingOptions) (*reddit.PostWithComments, error) {
	*s.hits++
	return &reddit.PostWithComments{}, s.err
}

func TestChainFallsThrough(t *testing.T) {
	var h1, h2 int
	c := chainFetcher{fetchers: []server.Fetcher{
		stubF{err: errors.New("403"), hits: &h1},
		stubF{page: &reddit.Page{After: "ok"}, hits: &h2},
	}}
	p, err := c.Subreddit(context.Background(), "golang", "hot", reddit.ListingOptions{})
	if err != nil || p.After != "ok" {
		t.Fatalf("fallthrough => %+v %v", p, err)
	}
	if h1 != 1 || h2 != 1 {
		t.Errorf("hits: primary=%d fallback=%d", h1, h2)
	}
	// Front + Comments fall through too.
	if _, err := c.Frontpage(context.Background(), "hot", reddit.ListingOptions{}); err != nil {
		t.Error(err)
	}
	if _, err := c.Comments(context.Background(), "golang", "abc", reddit.ListingOptions{}); err != nil {
		t.Error(err)
	}
}

func TestChainAllFail(t *testing.T) {
	var h int
	c := chainFetcher{fetchers: []server.Fetcher{stubF{err: errors.New("boom"), hits: &h}}}
	if _, err := c.Subreddit(context.Background(), "x", "hot", reddit.ListingOptions{}); err == nil {
		t.Error("want error when all fail")
	}
	if _, err := c.Frontpage(context.Background(), "hot", reddit.ListingOptions{}); err == nil {
		t.Error("front want error")
	}
	if _, err := c.Comments(context.Background(), "x", "y", reddit.ListingOptions{}); err == nil {
		t.Error("comments want error")
	}
}

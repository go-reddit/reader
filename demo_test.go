package main

import (
	"context"
	"testing"

	"github.com/go-reddit/reddit"
)

func TestDemoFetcher(t *testing.T) {
	d := demoFetcher{}
	ctx := context.Background()

	// A known subreddit returns its themed set.
	page, err := d.Subreddit(ctx, "golang", "hot", reddit.ListingOptions{})
	if err != nil || len(page.Posts) == 0 {
		t.Fatalf("golang: %d posts, %v", len(page.Posts), err)
	}
	if page.Posts[0].Subreddit != "golang" {
		t.Errorf("subreddit = %q", page.Posts[0].Subreddit)
	}

	// Different subreddits yield different content (the sidebar-switch fix).
	rust, _ := d.Subreddit(ctx, "rust", "hot", reddit.ListingOptions{})
	if rust.Posts[0].Title == page.Posts[0].Title {
		t.Error("golang and rust should differ in demo mode")
	}

	// An unknown subreddit is synthesised (name-specific).
	unknown, _ := d.Subreddit(ctx, "obscuresub", "hot", reddit.ListingOptions{})
	if len(unknown.Posts) == 0 || unknown.Posts[0].Subreddit != "obscuresub" {
		t.Errorf("unknown sub => %+v", unknown.Posts)
	}

	// Front page uses the "" key.
	front, err := d.Frontpage(ctx, "hot", reddit.ListingOptions{})
	if err != nil || len(front.Posts) == 0 || front.Posts[0].Subreddit != "popular" {
		t.Errorf("frontpage => %+v %v", front.Posts, err)
	}

	// Comments returns a stub tree.
	c, err := d.Comments(ctx, "golang", "abc", reddit.ListingOptions{})
	if err != nil || c.Post.ID != "abc" || len(c.Comments) != 1 {
		t.Errorf("comments => %+v %v", c, err)
	}
}

package main

import (
	"encoding/json"
	"testing"

	"github.com/go-reddit/reddit"
)

func TestSortOrHot(t *testing.T) {
	if sortOrHot("") != "hot" || sortOrHot(reddit.SortTop) != "top" {
		t.Error("sortOrHot")
	}
}

func TestListingQueryBuild(t *testing.T) {
	if listingQuery(reddit.ListingOptions{}) != "" {
		t.Error("empty opts should give empty query")
	}
	q := listingQuery(reddit.ListingOptions{Limit: 25, After: "t3_x", Time: reddit.TimeWeek})
	for _, want := range []string{"limit=25", "after=t3_x", "t=week"} {
		if !contains(q, want) {
			t.Errorf("query %q missing %q", q, want)
		}
	}
}

func TestEngineListingDecode(t *testing.T) {
	// The engine posts {posts:[<reddit post-data>], after}; it must decode
	// straight into reddit.Post via the struct tags.
	body := `{"id":"r1","after":"t3_next","posts":[
		{"id":"a1","name":"t3_a1","title":"Hello","author":"alice","subreddit":"golang","score":42,"num_comments":7,"permalink":"/r/golang/comments/a1/x/","domain":"go.dev","link_flair_text":"News"}
	]}`
	var r engineListing
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	if r.After != "t3_next" || len(r.Posts) != 1 {
		t.Fatalf("decoded = %+v", r)
	}
	p := r.Posts[0]
	if p.Title != "Hello" || p.Author != "alice" || p.Score != 42 || p.NumComments != 7 || p.Flair != "News" {
		t.Errorf("post = %+v", p)
	}
	if p.FullPermalink() != "https://www.reddit.com/r/golang/comments/a1/x/" {
		t.Errorf("permalink = %q", p.FullPermalink())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

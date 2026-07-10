package main

import (
	"context"

	"github.com/go-reddit/reddit"
)

// demoFetcher implements server.Fetcher with a fixed set of posts so the app
// can be demonstrated (and verified) without Reddit credentials or network
// access. Enabled by the -demo flag.
type demoFetcher struct{}

// demoPosts is a small, realistic feed used by -demo mode.
var demoPosts = []reddit.Post{
	{ID: "a1", Title: "Go 1.26 is released — generics, faster GC, and a new pure-Go wasm pipeline", Author: "golang_team", Subreddit: "golang", Domain: "go.dev", Score: 3421, NumComments: 287, Permalink: "/r/golang/comments/a1/go126/", Flair: "News"},
	{ID: "a2", Title: "Show reddit: a native macOS Reddit reader built with go-widgets and a purego WKWebView (CGO=0)", Author: "tannevaled", Subreddit: "golang", Domain: "github.com", Score: 1289, NumComments: 143, Permalink: "/r/golang/comments/a2/show/", Flair: "Show"},
	{ID: "a3", Title: "Why I stopped using CGO for macOS system integration", Author: "purist_dev", Subreddit: "golang", Domain: "blog.example.com", Score: 842, NumComments: 96, Permalink: "/r/golang/comments/a3/cgo/"},
	{ID: "a4", Title: "Benchmarking pure-Go SIMD across all six 64-bit architectures", Author: "asm_wizard", Subreddit: "golang", Domain: "self.golang", Score: 654, NumComments: 51, Permalink: "/r/golang/comments/a4/simd/", Flair: "Benchmark"},
	{ID: "a5", Title: "A tiny embeddable HTTP proxy pattern for wasm front-ends that need to dodge CORS", Author: "webby", Subreddit: "golang", Domain: "gist.github.com", Score: 421, NumComments: 38, Permalink: "/r/golang/comments/a5/proxy/"},
	{ID: "a6", Title: "TIL you can drive AppKit and WebKit from Go with zero cgo using the Objective-C runtime", Author: "til_poster", Subreddit: "golang", Domain: "developer.apple.com", Score: 388, NumComments: 44, Permalink: "/r/golang/comments/a6/til/", Flair: "TIL"},
	{ID: "a7", Title: "go-widgets: 62 pixel-blitting widgets, 100% test coverage, GTK4 + DaisyUI parity", Author: "widget_maker", Subreddit: "golang", Domain: "github.com", Score: 297, NumComments: 29, Permalink: "/r/golang/comments/a7/widgets/", Flair: "Show"},
	{ID: "a8", Title: "How the Reddit JSON API rate-limits anonymous traffic (and what a good User-Agent buys you)", Author: "api_archaeologist", Subreddit: "golang", Domain: "reddit.com", Score: 214, NumComments: 61, Permalink: "/r/golang/comments/a8/ratelimit/"},
}

func (demoFetcher) Subreddit(_ context.Context, name string, _ reddit.Sort, _ reddit.ListingOptions) (*reddit.Page, error) {
	posts := make([]reddit.Post, len(demoPosts))
	copy(posts, demoPosts)
	for i := range posts {
		if name != "" {
			posts[i].Subreddit = name
		}
	}
	return &reddit.Page{Posts: posts, After: ""}, nil
}

func (demoFetcher) Frontpage(ctx context.Context, sort reddit.Sort, opts reddit.ListingOptions) (*reddit.Page, error) {
	return demoFetcher{}.Subreddit(ctx, "", sort, opts)
}

func (demoFetcher) Comments(_ context.Context, _, id string, _ reddit.ListingOptions) (*reddit.PostWithComments, error) {
	return &reddit.PostWithComments{
		Post:     reddit.Post{ID: id, Title: "Demo post", Author: "demo"},
		Comments: []reddit.Comment{{ID: "c1", Author: "commenter", Body: "This is a demo comment tree.", Score: 12}},
	}, nil
}

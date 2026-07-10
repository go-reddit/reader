package server

import (
	"embed"
	"io/fs"
)

// embedded holds the front-end bundle. index.html and wasm_exec.js are
// committed; reader.wasm is produced by `scripts/build-wasm.sh` (gitignored)
// before a real build. The `all:` prefix keeps dotfiles like .gitkeep so the
// embed succeeds even on a fresh checkout that hasn't built the wasm yet.
//
//go:embed all:assets
var embedded embed.FS

// Assets is the front-end file system rooted at the bundle (so paths are
// "index.html", not "assets/index.html"). Pass it to [New].
var Assets = mustSub(embedded, "assets")

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err) // only fails if the embed directive and dir disagree
	}
	return sub
}

package ui

import (
	"sync"

	"github.com/go-opentype/fonts/goregular"
	"github.com/go-opentype/fonts/notoemoji"
	"github.com/go-opentype/fonts/notosansarabic"
	"github.com/go-opentype/fonts/notosansarmenian"
	"github.com/go-opentype/fonts/notosansdevanagari"
	"github.com/go-opentype/fonts/notosansgeorgian"
	"github.com/go-opentype/fonts/notosanshebrew"
	"github.com/go-opentype/fonts/notosansjp"
	"github.com/go-opentype/fonts/notosanskr"
	"github.com/go-opentype/fonts/notosanssc"
	"github.com/go-opentype/fonts/notosansthai"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// This file gives the reader real, anti-aliased, SHAPED text. There is exactly
// one text stack: every string the reader measures or paints goes through a
// go-widgets toolkit font, which shapes the run with github.com/go-opentype/shape.
//
// The reader used to carry a second, private rasteriser over golang.org/x/image
// /font/gofont: it routed runes to faces itself and drew them one at a time. That
// path applied no GSUB at all, so it measured and drew Arabic in unjoined isolated
// forms, left Indic clusters unreordered, could not compose an emoji ZWJ sequence,
// and — since it only carried the Go Latin faces — rendered CJK / Arabic / Indic /
// emoji as empty .notdef boxes. textFace is now a thin adapter over the toolkit
// font, keeping the top-left-corner positioning its callers expect.

// faceKey identifies a cached font by its pixel size and weight.
type faceKey struct {
	px   int
	bold bool
}

// ttCache memoises the built font chains (keyed by size + weight): parsing ten
// faces, one of which is a 17 MB CJK Noto, is far too costly to redo per frame.
// Guarded by ttMu.
var (
	ttMu    sync.Mutex
	ttCache = map[faceKey]toolkit.Font{}
)

// scriptFallbackTTFs are the per-script Noto faces chained after the Latin UI
// font so the reader can display text in any of them — no single font covers
// every script, and Noto is split per script, so mixed text (e.g. a Chinese or
// Arabic title next to Latin) needs per-grapheme font routing. go-opentype
// parses each lazily (~1ms), and go:embed shares one copy of the bytes, so the
// chain is cheap in time and memory despite the large CJK face.
//
// Noto Emoji is LAST, and must stay last. Routing sends a rune to the FIRST face
// that covers it, so a face this late can only ever claim runes every text face
// declined. That matters here because the emoji family does map a handful of
// ASCII — "#", "*" and the digits, the bases of the keycap emoji sequences —
// which a text face must keep serving.
var scriptFallbackTTFs = [][]byte{
	notosanssc.TTF,         // Chinese + Han ideographs + kana
	notosansjp.TTF,         // Japanese (kana + JP kanji forms)
	notosanskr.TTF,         // Korean (Hangul)
	notosansthai.TTF,       // Thai
	notosansarabic.TTF,     // Arabic
	notosansdevanagari.TTF, // Devanagari (Hindi, …)
	notosanshebrew.TTF,     // Hebrew
	notosansarmenian.TTF,   // Armenian
	notosansgeorgian.TTF,   // Georgian
	notoemoji.TTF,          // emoji + pictographs — keep LAST, see above
}

// ttFont returns the cached shaped font at px pixels, regular or bold: the
// embedded Go primary followed by the per-script Noto faces, chained through
// toolkit.NewFallbackFont so glyphs the primary lacks render from the face that
// has them. Assign it to a widget's Base.Font, or wrap it in a textFace to draw
// by the text's top-left corner.
func ttFont(bold bool, px int) toolkit.Font {
	ttMu.Lock()
	defer ttMu.Unlock()
	k := faceKey{px, bold}
	if f, ok := ttCache[k]; ok {
		return f
	}
	f := buildFont(bold, px)
	ttCache[k] = f
	return f
}

// buildFont assembles one chain: the Go Latin primary (a real bold weight ships,
// so no synthetic emboldening is needed) followed by the Noto fallback faces.
func buildFont(bold bool, px int) toolkit.Font {
	src := goregular.TTF
	if bold {
		src = goregular.BoldTTF
	}
	// The Go faces always parse, so the chain always has a valid first font.
	primary, _ := toolkit.NewTrueTypeFont(src, px)
	fonts := []toolkit.Font{primary}
	for _, ttf := range scriptFallbackTTFs {
		if fb, err := toolkit.NewTrueTypeFont(ttf, px); err == nil {
			fonts = append(fonts, fb)
		}
	}
	// NewFallbackFont never errors when the first font is valid, as it is here.
	f, _ := toolkit.NewFallbackFont(fonts...)
	return f
}

// textFace adapts a toolkit font to the reader's drawing convention: callers
// position by the line's TOP-LEFT corner, and size their rows by the line
// height, so the height is cached alongside the font. The baseline is the font's
// own business — Draw derives it — so this carries no ascent.
type textFace struct {
	font   toolkit.Font
	height int
	px     int // the pixel size this face was built at
}

// getFace returns the face at px pixels, regular or bold. It is the same cached
// font ttFont hands to widgets, so a string measured here and drawn by a widget
// (or vice versa) can never disagree.
func getFace(px int, bold bool) textFace {
	if px < 1 {
		px = 1
	}
	f := ttFont(bold, px)
	return textFace{font: f, height: f.Height(), px: px}
}

// width is the rendered pixel width of s, shaped: ligatures, cursive joining and
// kerning are all reflected, so it matches what a Label paints exactly (a Label
// built with this face measures the run through the very same font).
func (tf textFace) width(s string) int { return tf.font.Measure(s) }

// labelAt paints s as a stock toolkit.Label carrying this face, its top-left at
// (x, top). There is no private glyph-blit any more: every string the reader
// draws goes through a real widget, so a11y walkers, selection and future
// theming all see it. The bounds are exactly one glyph-row tall and top-anchored
// so the Label lands where the old top-left convention put it.
func (tf textFace) labelAt(p painter.Painter, th *toolkit.Theme, x, top int, s string, ink toolkit.RGBA) {
	l := toolkit.NewLabel(s)
	l.Font, l.Ink, l.VAlign = tf.font, ink, toolkit.VTop
	l.SetBounds(toolkit.Rect{X: x, Y: top, W: tf.width(s), H: tf.height})
	l.Draw(p, th)
}

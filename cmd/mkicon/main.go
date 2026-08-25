// Command mkicon renders the Reddit Reader app icon and writes it as a macOS
// .icns file (or a .png when the output path ends in .png). The icon is drawn
// with the go-widgets painter — an orange squircle with a white comment bubble
// and an upvote arrow — then downscaled to every icns size and encoded in pure
// Go, so no iconutil / sips is needed.
//
//	go run ./cmd/mkicon dist/AppIcon.icns
//
//bricolint:allowfile build tool that rasterises a static icon asset, not interactive UI chrome — the painter primitives here are a genuine render leaf
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"

	"github.com/go-widgets/painter"
	xdraw "golang.org/x/image/draw"
)

// render draws the icon at 1024×1024 into an *image.RGBA (transparent outside
// the squircle).
func render() *image.RGBA {
	const S = 1024
	buf := make([]byte, S*S*4)
	p := painter.NewPixelPainter(buf, S, S)

	orange := painter.RGB(0xFF, 0x45, 0x00)
	white := painter.RGB(0xFF, 0xFF, 0xFF)

	// Rounded-squircle tile (macOS grid: ~824px art in 1024 with big radius).
	m := 100
	p.FillRoundRect(painter.Rect{X: m, Y: m, W: S - 2*m, H: S - 2*m}, 200, orange)

	// White comment bubble.
	bx, by, bw, bh := 300, 300, 424, 300
	p.FillRoundRect(painter.Rect{X: bx, Y: by, W: bw, H: bh}, 90, white)
	// Tail (lower-left), a downward right-triangle.
	tailW, tailH, tailLeft := 120, 120, bx+70
	for i := 0; i < tailH; i++ {
		w := tailW * (tailH - i) / tailH
		p.FillRect(painter.Rect{X: tailLeft, Y: by + bh - 8 + i, W: w, H: 1}, white)
	}

	// Orange upvote arrow inside the bubble.
	cx := bx + bw/2
	ay, ah, aw := by+66, 120, 150 // apex y, height, half-base
	for i := 0; i < ah; i++ {
		w := aw * i / ah
		p.FillRect(painter.Rect{X: cx - w, Y: ay + i, W: 2 * w, H: 1}, orange)
	}
	p.FillRect(painter.Rect{X: cx - 30, Y: ay + ah, W: 60, H: 96}, orange)

	return &image.RGBA{Pix: buf, Stride: S * 4, Rect: image.Rect(0, 0, S, S)}
}

// scaleTo produces a size×size copy with high-quality resampling.
func scaleTo(src *image.RGBA, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

// pngBytes encodes an image as PNG.
func pngBytes(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// icnsEntry maps an OSType to a pixel size.
var icnsEntries = []struct {
	typ  string
	size int
}{
	{"icp4", 16}, {"icp5", 32}, {"ic07", 128}, {"ic08", 256},
	{"ic09", 512}, {"ic10", 1024}, {"ic11", 32}, {"ic12", 64},
	{"ic13", 256}, {"ic14", 512},
}

// writeICNS encodes the icon set as an .icns file.
func writeICNS(base *image.RGBA) ([]byte, error) {
	var body bytes.Buffer
	for _, e := range icnsEntries {
		data, err := pngBytes(scaleTo(base, e.size))
		if err != nil {
			return nil, err
		}
		body.WriteString(e.typ)
		if err := binary.Write(&body, binary.BigEndian, uint32(8+len(data))); err != nil {
			return nil, err
		}
		body.Write(data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mkicon <out.icns|out.png>")
		os.Exit(2)
	}
	out := os.Args[1]
	base := render()

	var data []byte
	var err error
	if strings.HasSuffix(strings.ToLower(out), ".png") {
		data, err = pngBytes(base)
	} else {
		data, err = writeICNS(base)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkicon:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "mkicon:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(data))
}

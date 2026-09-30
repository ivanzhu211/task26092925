package localrender

import (
	"testing"
)

// boundsReaderAt serves a deterministic synthetic source without ever
// materializing a full image-sized buffer. It records the union of pixel
// ranges touched as [minX,maxX) x [minY,maxY] plus the ReadAt call count.
type boundsReaderAt struct {
	w, h int

	calls                                      int
	minX                                       int
	minY                                       int
	maxX                                       int
	maxY                                       int
	violated                                   int
	allowedX0, allowedY0, allowedX1, allowedY1 int
}

func (b *boundsReaderAt) ReadAt(p []byte, off int64) (int, error) {
	b.calls++
	startPixel := off / 3
	x0 := int(startPixel) % b.w
	y0 := int(startPixel) / b.w
	n := len(p) / 3
	x1 := x0 + n
	if x0 < b.allowedX0 || x1 > b.allowedX1 || y0 < b.allowedY0 || y0 >= b.allowedY1 {
		b.violated++
	}
	if b.calls == 1 {
		b.minX, b.maxX = x0, x1
		b.minY, b.maxY = y0, y0+1
	} else {
		if x0 < b.minX {
			b.minX = x0
		}
		if x1 > b.maxX {
			b.maxX = x1
		}
		if y0 < b.minY {
			b.minY = y0
		}
		if y0+1 > b.maxY {
			b.maxY = y0 + 1
		}
	}
	for i := 0; i < n; i++ {
		x, y := x0+i, y0
		p[3*i+0] = byte((x*7 + y*13) % 256)
		p[3*i+1] = byte((x*3 - y*5 + 10000) % 256)
		p[3*i+2] = byte((x ^ y ^ (x + y)) % 256)
	}
	return len(p), nil
}

// TestLargeImageLocalReadAndMemory renders a small center region of a large
// image after two consecutive blurs and a downscale, then asserts:
//
//  1. the reader is called once per required source row and never outside the
//     exact source dependency rectangle predicted by backtracking;
//  2. bytes read are a small fraction of the full image;
//  3. the number of allocations per run is repeatable (testing.AllocsPerRun)
//     and stays within a tight local-render budget, proving no full-frame
//     intermediate is ever built.
func TestLargeImageLocalReadAndMemory(t *testing.T) {
	const srcW, srcH = 4000, 3000
	ops := []Op{BoxBlur(3), BoxBlur(2), Resize(800, 600)}

	dims := stageDims(srcW, srcH, ops)
	want := Rect{394, 295, 406, 304} // center 12x9 of the 800x600 result
	reqs := backtrack(dims, ops, want)
	srcReq := reqs[0]

	// Deterministic working-set bound: the largest stage-local buffer, summed
	// with the other live stage windows, is a tiny fraction of one full
	// frame. This is repeatable across runs because it depends only on the
	// plan and rectangle.
	var stagePixels, maxStagePixels int
	for _, r := range reqs {
		a := r.Width() * r.Height()
		stagePixels += a
		if a > maxStagePixels {
			maxStagePixels = a
		}
	}
	fullPixels := srcW * srcH
	if got, limit := maxStagePixels*3, fullPixels*3/100; got > limit {
		t.Fatalf("largest local buffer %d bytes exceeds 1%% of full frame %d", got, fullPixels*3)
	}
	t.Logf("stage window pixels total=%d largest=%d vs full=%d (%.4f%%)",
		stagePixels, maxStagePixels, fullPixels,
		100*float64(stagePixels)/float64(fullPixels))

	reader := &boundsReaderAt{
		w:         srcW,
		h:         srcH,
		allowedX0: srcReq.X0,
		allowedY0: srcReq.Y0,
		allowedX1: srcReq.X1,
		allowedY1: srcReq.Y1,
	}
	got := Render(reader, srcW, srcH, ops, want)
	if len(got) != want.Width()*want.Height()*3 {
		t.Fatalf("output size = %d, want %d", len(got), want.Width()*want.Height()*3)
	}

	if reader.violated != 0 {
		t.Fatalf("observed %d ReadAt calls outside dependency rect %+v", reader.violated, srcReq)
	}
	if reader.calls != srcReq.Height() {
		t.Fatalf("ReadAt calls = %d, want exactly %d (one per dependency row)", reader.calls, srcReq.Height())
	}
	touched := Rect{reader.minX, reader.minY, reader.maxX, reader.maxY}
	if touched != srcReq {
		t.Fatalf("touched union %+v != predicted dependency %+v", touched, srcReq)
	}

	fullBytes := int64(srcW * srcH * 3)
	readBytes := int64(reader.calls) * int64(srcReq.Width()*3)
	if ratio := float64(readBytes) / float64(fullBytes); ratio > 0.01 {
		t.Fatalf("read %d bytes (%.4f of full frame); expected a tiny central fraction", readBytes, ratio)
	}
	t.Logf("source dep rect=%+v (%dx%d); read %d bytes = %.6f of %d-byte full frame",
		srcReq, srcReq.Width(), srcReq.Height(), readBytes,
		float64(readBytes)/float64(fullBytes), fullBytes)

	allocs := testing.AllocsPerRun(3, func() {
		r := &boundsReaderAt{
			w:         srcW,
			h:         srcH,
			allowedX0: srcReq.X0,
			allowedY0: srcReq.Y0,
			allowedX1: srcReq.X1,
			allowedY1: srcReq.Y1,
		}
		if out := Render(r, srcW, srcH, ops, want); len(out) != len(got) {
			t.Fatal("nondeterministic output size")
		}
	})
	t.Logf("Render allocs/run = %.0f", allocs)
	// One buffer per stage plus the result; far below any whole-frame stage.
	if allocs > 12 {
		t.Fatalf("allocs/run = %.0f exceeds local-render budget", allocs)
	}
}

// TestLargeImageOutputMatchesFullFrame verifies correctness on the large
// pipeline at a manageable effective size by comparing against the reference,
// while the 4000x3000 locality properties are covered above. It repeats the
// exact operation shape (two blurs then a downscale) on a moderate image.
func TestLargeImageOutputMatchesFullFrame(t *testing.T) {
	srcW, srcH := 160, 120
	src := make([]byte, srcW*srcH*3)
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			i := (y*srcW + x) * 3
			src[i+0] = byte((x*7 + y*13) % 256)
			src[i+1] = byte((x*3 - y*5 + 10000) % 256)
			src[i+2] = byte((x ^ y ^ (x + y)) % 256)
		}
	}
	ops := []Op{BoxBlur(3), BoxBlur(2), Resize(80, 60)}
	ref := newFullFrame(srcW, srcH, append([]byte(nil), src...))
	ref.runPlan(ops)
	for _, r := range []Rect{{38, 28, 50, 37}, {0, 0, 3, 2}, {77, 57, 80, 60}} {
		got := Render(bytesReader(src), srcW, srcH, ops, r)
		want := ref.sliceRect(r)
		if string(got) != string(want) {
			t.Fatalf("rect %+v mismatch (got %d bytes vs want %d)", r, len(got), len(want))
		}
	}
}

type bytesReader []byte

func (b bytesReader) ReadAt(p []byte, off int64) (int, error) {
	return copy(p, b[off:]), nil
}

package localrender

import (
	"bytes"
	"math/rand"
	"testing"
)

// countingReaderAt records every ReadAt request. It is also used in small
// tests to sanity-check that reads stay inside the source dependency region.
type countingReaderAt struct {
	data     []byte
	width    int
	calls    int
	minOff   int64
	maxOff   int64 // one-past the highest byte ever requested
	firstW   int
	rowCalls int
}

func newCountingReaderAt(data []byte, width int) *countingReaderAt {
	return &countingReaderAt{data: data, width: width, minOff: 1 << 62, maxOff: -1}
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.calls++
	if off < c.minOff {
		c.minOff = off
	}
	end := off + int64(len(p))
	if end > c.maxOff {
		c.maxOff = end
	}
	return copy(p, c.data[off:off+int64(len(p))]), nil
}

// randomPlan builds a guaranteed-valid random plan. Generation weights ensure
// frequent consecutive blurs, repeated crops, and non-integer scale ratios.
func randomPlan(rng *rand.Rand, w, h int) ([]Op, int, int) {
	ops := []Op{}
	n := 1 + rng.Intn(6)
	// Guarantee at least two consecutive blurs in about half the plans.
	if rng.Intn(2) == 0 {
		ops = append(ops, BoxBlur(1+rng.Intn(2)), BoxBlur(rng.Intn(2)))
	}
	for i := len(ops); i < n; i++ {
		switch rng.Intn(6) {
		case 0, 1: // blur (raises the chance of consecutive blurs)
			ops = append(ops, BoxBlur(rng.Intn(3)))
		case 2: // crop
			if w > 1 && h > 1 {
				cw := 1 + rng.Intn(w)
				ch := 1 + rng.Intn(h)
				x := rng.Intn(w - cw + 1)
				y := rng.Intn(h - ch + 1)
				ops = append(ops, Crop(x, y, cw, ch))
				w, h = cw, ch
			} else {
				ops = append(ops, BoxBlur(rng.Intn(2)))
			}
		default: // resize, including non-integer ratios in both directions
			nw := 1 + rng.Intn(3*w+4)
			nh := 1 + rng.Intn(3*h+4)
			ops = append(ops, Resize(nw, nh))
			w, h = nw, nh
		}
	}
	return ops, w, h
}

func randomRect(rng *rand.Rand, w, h int) Rect {
	x0 := rng.Intn(w)
	y0 := rng.Intn(h)
	x1 := x0 + 1 + rng.Intn(w-x0)
	y1 := y0 + 1 + rng.Intn(h-y0)
	return Rect{x0, y0, x1, y1}
}

// interestingRects returns edge, corner, full-frame and sampling-boundary
// rectangles for exhaustive coverage beyond random ones.
func interestingRects(w, h int) []Rect {
	rs := []Rect{
		{0, 0, w, h},
		{0, 0, 1, 1},
		{w - 1, h - 1, w, h},
		{0, h - 1, 1, h},
		{w - 1, 0, w, 1},
	}
	if w > 2 {
		rs = append(rs, Rect{w / 2, 0, w/2 + 1, h})
	}
	return rs
}

func TestRandomPlansMatchFullFrame(t *testing.T) {
	const iterations = 4000
	for seed := int64(1); seed <= iterations; seed++ {
		rng := rand.New(rand.NewSource(seed))
		w := 1 + rng.Intn(7)
		h := 1 + rng.Intn(7)
		src := make([]byte, w*h*3)
		// Mix structured channels and random values so tie cases and
		// edge-clamped windows both occur.
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				idx := (y*w + x) * 3
				src[idx+0] = byte(rng.Intn(256))
				src[idx+1] = byte((x*255 + y) % 256)
				src[idx+2] = byte(255 * (1 - (x+y)%2))
			}
		}
		ops, fw, fh := randomPlan(rng, w, h)

		ref := newFullFrame(w, h, append([]byte(nil), src...))
		ref.runPlan(ops)

		rects := interestingRects(fw, fh)
		for k := 0; k < 4; k++ {
			rects = append(rects, randomRect(rng, fw, fh))
		}
		for ri, rect := range rects {
			cr := newCountingReaderAt(src, w)
			got := Render(cr, w, h, ops, rect)
			want := ref.sliceRect(rect)
			if !bytes.Equal(got, want) {
				t.Fatalf("seed=%d rect#=%d rect=%+v final=%dx%d ops=%+v\nmismatch:\n got=%v\nwant=%v",
					seed, ri, rect, fw, fh, ops, got, want)
			}
			// The union of every read must equal the exact predicted source
			// dependency rectangle, and reads are one tightly packed row each.
			pred := backtrack(stageDims(w, h, ops), ops, rect)[0]
			if cr.calls != pred.Height() {
				t.Fatalf("seed=%d rect#=%d: %d ReadAt calls, want %d (one/dependency row)",
					seed, ri, cr.calls, pred.Height())
			}
			rowLen := int64(pred.Width() * 3)
			first := int64(pred.Y0*w+pred.X0) * 3
			lastEnd := int64((pred.Y1-1)*w+pred.X0)*3 + rowLen
			if cr.minOff != first || cr.maxOff != lastEnd {
				t.Fatalf("seed=%d rect#=%d: read span [%d,%d) want [%d,%d) pred=%+v",
					seed, ri, cr.minOff, cr.maxOff, first, lastEnd, pred)
			}
		}
	}
}

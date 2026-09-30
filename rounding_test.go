package localrender

import (
	"bytes"
	"testing"
)

// TestResizeHalfTieRoundsUp forces exact half-integer values through bilinear
// resampling and checks they round upward (127.5 -> 128, not 127).
//
// Resampling width 2 -> 3 gives destination center x=1 source position
// exactly 0.5, so its two horizontal neighbors each carry weight 1/2. With
// source pixels 0 and 255, the interpolated value is 127.5 for one channel.
func TestResizeHalfTieRoundsUp(t *testing.T) {
	src := []byte{
		0, 0, 0, 255, 255, 255,
	}
	ops := []Op{Resize(3, 1)}
	got := Render(bytes.NewReader(src), 2, 1, ops, Rect{0, 0, 3, 1})

	// x=0 has source position -1/6, whose left neighbor clamps to pixel 0
	// (weight 7/6 vs -1/6), so it stays 0. x=1 is the exact 0.5 tie and must
	// round up to 128. x=2 is the symmetric right edge and stays 255.
	want := []byte{
		0, 0, 0,
		128, 128, 128,
		255, 255, 255,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("half-tie rounding mismatch:\n got=%v\nwant=%v", got, want)
	}
}

// TestHalfTieAcrossBothAxes exercises an exact tie in the two-dimensional
// combination (1/2 x 1/2 weights), and a 127/128 pair hitting 127.5 exactly.
func TestHalfTieAcrossBothAxes(t *testing.T) {
	src := make([]byte, 2*2*3)
	src[3*0] = 0   // (0,0) R
	src[3*1] = 255 // (1,0) R
	src[3*2] = 0   // (0,1) R
	src[3*3] = 255 // (1,1) R
	ops := []Op{Resize(3, 3)}
	got := Render(bytes.NewReader(src), 2, 2, ops, Rect{1, 1, 2, 2})
	if got[0] != 128 {
		t.Fatalf("2D half tie: got center R=%d, want 128 (full=%v)", got[0], got)
	}
}

// TestConsecutiveBlursEdge confirms consecutive blurs including radius 0,
// radius 1, edge pixels, and full-window denominator match the full-frame
// reference on a hand-built pattern.
func TestConsecutiveBlursEdge(t *testing.T) {
	w, h := 5, 4
	src := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 3
			src[i] = byte((x * 50) % 256)
			src[i+1] = byte((y * 80) % 256)
			src[i+2] = byte((x + y) % 2 * 255)
		}
	}
	ops := []Op{BoxBlur(1), BoxBlur(2), BoxBlur(0), BoxBlur(1)}
	ref := newFullFrame(w, h, append([]byte(nil), src...))
	ref.runPlan(ops)
	rects := []Rect{
		{0, 0, w, h},
		{0, 0, 1, 1},
		{w - 1, h - 1, w, h},
		{2, 1, 4, 3},
	}
	for _, r := range rects {
		got := Render(bytes.NewReader(src), w, h, ops, r)
		want := ref.sliceRect(r)
		if !bytes.Equal(got, want) {
			t.Fatalf("blur rect %+v mismatch:\n got=%v\nwant=%v", r, got, want)
		}
	}
}

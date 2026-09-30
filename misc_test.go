package localrender

import (
	"bytes"
	"testing"
)

// TestNoOpsIdentity verifies that a plan with no operations returns the
// requested source rectangle verbatim.
func TestNoOpsIdentity(t *testing.T) {
	w, h := 3, 2
	src := []byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9,
		10, 11, 12, 13, 14, 15, 16, 17, 18,
	}
	r := Rect{1, 0, 3, 2}
	got := Render(bytes.NewReader(src), w, h, nil, r)
	want := []byte{
		4, 5, 6, 7, 8, 9,
		13, 14, 15, 16, 17, 18,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("no-op identity mismatch:\n got=%v\nwant=%v", got, want)
	}
}

// TestSourceNeverMutated keeps a copy of the source and proves Render never
// writes through the read-only source.
func TestSourceNeverMutated(t *testing.T) {
	w, h := 6, 5
	src := make([]byte, w*h*3)
	for i := range src {
		src[i] = byte((i*37 + 11) % 256)
	}
	snapshot := append([]byte(nil), src...)
	ops := []Op{
		BoxBlur(2),
		Crop(1, 1, 4, 3),
		Resize(7, 6),
		BoxBlur(1),
	}
	_ = Render(bytes.NewReader(src), w, h, ops, Rect{0, 0, 7, 6})
	if !bytes.Equal(src, snapshot) {
		t.Fatal("source bytes were modified by Render")
	}
}

// TestCropCannotReuseRemovedPixels builds a crop followed by blur/resize and
// compares with the reference, ensuring cropped-away pixels never influence
// later edge clamping.
func TestCropCannotReuseRemovedPixels(t *testing.T) {
	w, h := 8, 6
	src := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 3
			src[i] = byte(x * 30)
			src[i+1] = byte(y * 45)
			src[i+2] = byte((x + y*2) % 256)
		}
	}
	ops := []Op{Crop(2, 1, 4, 4), BoxBlur(2), Resize(9, 7), BoxBlur(1)}
	ref := newFullFrame(w, h, append([]byte(nil), src...))
	ref.runPlan(ops)
	for _, r := range []Rect{{0, 0, 9, 7}, {0, 0, 2, 2}, {7, 5, 9, 7}, {3, 2, 6, 5}} {
		got := Render(bytes.NewReader(src), w, h, ops, r)
		want := ref.sliceRect(r)
		if !bytes.Equal(got, want) {
			t.Fatalf("post-crop rect %+v mismatch", r)
		}
	}
}

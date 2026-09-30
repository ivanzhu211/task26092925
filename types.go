// Package localrender performs non-destructive, region-local rendering of a
// planned sequence of photo edits.
//
// Source images are tightly packed row-major RGB8 bytes (3 bytes per pixel,
// no stride, no header). The three channels are processed independently and
// quantized to 8-bit integers only at the end of each operation, rounding
// half-integers upward.
package localrender

// Op is one non-destructive edit operation in an ordered edit plan.
type Op interface {
	opTag() opTag
}

type opTag int

const (
	tagCrop opTag = iota
	tagResize
	tagBoxBlur
)

// CropOp keeps the rectangle starting at (X, Y) with W columns and H rows.
// The result origin is reset to (0, 0).
type CropOp struct {
	X, Y, W, H int
}

// ResizeOp resamples to W x H using pixel-center aligned bilinear
// interpolation with edge clamping.
type ResizeOp struct {
	W, H int
}

// BoxBlurOp applies a normalized (2R+1) x (2R+1) box window with
// clamped-to-edge out-of-bounds sampling.
type BoxBlurOp struct {
	R int
}

func (CropOp) opTag() opTag    { return tagCrop }
func (ResizeOp) opTag() opTag  { return tagResize }
func (BoxBlurOp) opTag() opTag { return tagBoxBlur }

// Crop constructs a crop operation.
func Crop(x, y, w, h int) CropOp { return CropOp{X: x, Y: y, W: w, H: h} }

// Resize constructs a resize operation.
func Resize(w, h int) ResizeOp { return ResizeOp{W: w, H: h} }

// BoxBlur constructs a box blur operation with radius r.
func BoxBlur(r int) BoxBlurOp { return BoxBlurOp{R: r} }

// Rect is a half-open rectangle [X0, X1) x [Y0, Y1).
type Rect struct {
	X0, Y0, X1, Y1 int
}

// Width returns the rectangle width.
func (r Rect) Width() int { return r.X1 - r.X0 }

// Height returns the rectangle height.
func (r Rect) Height() int { return r.Y1 - r.Y0 }

// Empty reports whether the rectangle contains no pixels.
func (r Rect) Empty() bool { return r.X1 <= r.X0 || r.Y1 <= r.Y0 }

// Intersect returns the intersection of r and s.
func (r Rect) Intersect(s Rect) Rect {
	out := Rect{
		X0: max(r.X0, s.X0),
		Y0: max(r.Y0, s.Y0),
		X1: min(r.X1, s.X1),
		Y1: min(r.Y1, s.Y1),
	}
	if out.Empty() {
		return Rect{}
	}
	return out
}

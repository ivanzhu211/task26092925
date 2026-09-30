package localrender

// stageDim is the width and height of the image at one pipeline stage.
type stageDim struct {
	W, H int
}

// stageDims reports the image dimensions at every pipeline stage. Stage 0 is
// the source image; stage i+1 is the image after ops[i].
func stageDims(srcW, srcH int, ops []Op) []stageDim {
	dims := make([]stageDim, len(ops)+1)
	dims[0] = stageDim{srcW, srcH}
	w, h := srcW, srcH
	for i, op := range ops {
		switch o := op.(type) {
		case CropOp:
			w, h = o.W, o.H
		case ResizeOp:
			w, h = o.W, o.H
		case BoxBlurOp:
			// Blur preserves dimensions.
		}
		dims[i+1] = stageDim{w, h}
	}
	return dims
}

// floorDiv returns floor(a/b) for b > 0, including negative a.
func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b) != 0 && a < 0 {
		q--
	}
	return q
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// backtrack maps a requested rectangle in the final image back through ops,
// returning the dependency rectangle at every stage. The returned slice has
// the same length as dims; element i is expressed in stage-i coordinates.
func backtrack(dims []stageDim, ops []Op, want Rect) []Rect {
	req := make([]Rect, len(dims))
	cur := want
	req[len(dims)-1] = cur
	for i := len(ops) - 1; i >= 0; i-- {
		d := dims[i]
		switch o := ops[i].(type) {
		case CropOp:
			cur = Rect{
				X0: cur.X0 + o.X,
				Y0: cur.Y0 + o.Y,
				X1: cur.X1 + o.X,
				Y1: cur.Y1 + o.Y,
			}
		case ResizeOp:
			// First output pixel reaches source index floor(s(X0)); the last
			// output pixel X1-1 touches floor(s(X1-1))+1, so the half-open
			// end runs one past that.
			cur = Rect{
				X0: resizeBackCoord(cur.X0, d.W, o.W),
				Y0: resizeBackCoord(cur.Y0, d.H, o.H),
				X1: resizeBackCoord(cur.X1-1, d.W, o.W) + 2,
				Y1: resizeBackCoord(cur.Y1-1, d.H, o.H) + 2,
			}
			cur = cur.Intersect(Rect{0, 0, d.W, d.H})
		case BoxBlurOp:
			cur = Rect{
				X0: cur.X0 - o.R,
				Y0: cur.Y0 - o.R,
				X1: cur.X1 + o.R,
				Y1: cur.Y1 + o.R,
			}.Intersect(Rect{0, 0, d.W, d.H})
		}
		req[i] = cur
	}
	return req
}

// resizeBackCoord returns the (unclamped) floored source-space coordinate
// sampled by destination pixel x under pixel-center aligned bilinear
// resampling from srcSize to dstSize: src = (x+0.5)*srcSize/dstSize - 0.5.
func resizeBackCoord(x, srcSize, dstSize int) int {
	num := int64(2*x+1)*int64(srcSize) - int64(dstSize)
	den := int64(2 * dstSize)
	return int(floorDiv(num, den))
}

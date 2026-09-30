package localrender

import "io"

// Render applies ops to the source image exposed by reader (width x height,
// tightly packed RGB8) and returns the requested rect of the final image as
// tightly packed row-major RGB8 bytes.
//
// The requested rectangle is first backtracked through the plan to derive the
// pixel dependency region at every stage; the source is read only inside that
// region and each intermediate image is computed only for its dependency
// region. The result is byte-identical to processing the full image through
// every operation and then cropping to rect. The source data is never
// modified.
func Render(reader io.ReaderAt, width, height int, ops []Op, rect Rect) []byte {
	dims := stageDims(width, height, ops)
	req := backtrack(dims, ops, rect)

	cur := readTile(reader, width, req[0])
	for i, op := range ops {
		want := req[i+1]
		switch o := op.(type) {
		case CropOp:
			cur = applyCrop(cur, o, want)
		case ResizeOp:
			cur = applyResize(cur, dims[i].W, dims[i].H, dims[i+1].W, dims[i+1].H, want)
		case BoxBlurOp:
			cur = applyBoxBlur(cur, o.R, want)
		}
	}
	return cur.Pix
}

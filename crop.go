package localrender

// applyCrop re-orients the already-loaded dependency window for a crop
// operation. Backtracking guarantees the input window is exactly the crop of
// the previous image required downstream (a crop neither expands nor resamples
// dependencies), so the pixel buffer is shared without copying; only the
// origin shifts into the post-crop coordinate system (pre-crop origin minus
// the crop offset, which also handles back-to-back crops).
func applyCrop(t tile, op CropOp, want Rect) tile {
	return tile{GX: t.GX - op.X, GY: t.GY - op.Y, W: t.W, H: t.H, Pix: t.Pix}
}

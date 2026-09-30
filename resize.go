package localrender

// bilinearAxis describes the exact sampling of one output coordinate under
// pixel-center aligned bilinear resampling.
type bilinearAxis struct {
	i0, i1 int   // left/top and right/bottom neighbor indices (pre-clamp)
	w0, w1 int64 // weights over denominator den
	den    int64
}

// bilinearX returns the exact horizontal weights for destination coordinate
// x when resampling srcSize -> dstSize. The source coordinate is
// src = (x+0.5)*srcSize/dstSize - 0.5.
func bilinearX(x, srcSize, dstSize int) bilinearAxis {
	num := int64(2*x+1)*int64(srcSize) - int64(dstSize)
	den := int64(2 * dstSize)
	i0 := int(floorDiv(num, den))
	w1 := num - int64(i0)*den // fractional numerator, 0 <= w1 < den
	w0 := den - w1
	return bilinearAxis{i0: i0, i1: i0 + 1, w0: w0, w1: w1, den: den}
}

// applyResize produces the want rectangle of a srcW x srcH image resampled to
// dstW x dstH using bilinear interpolation. Neighbor coordinates and weights
// are determined exactly first; out-of-range neighbor indices are then
// clamped to the current image edge. The weighted accumulation for the four
// neighbors is exact, and each channel is quantized once when the output
// pixel is finalized.
func applyResize(t tile, srcW, srcH, dstW, dstH int, want Rect) tile {
	out := tile{GX: want.X0, GY: want.Y0, W: want.Width(), H: want.Height()}
	out.Pix = make([]uint8, out.W*out.H*3)
	denom := int64(2*dstW) * int64(2*dstH)
	for oy := 0; oy < out.H; oy++ {
		gy := want.Y0 + oy
		ay := bilinearX(gy, srcH, dstH)
		y0 := clampInt(ay.i0, 0, srcH-1)
		y1 := clampInt(ay.i1, 0, srcH-1)
		for ox := 0; ox < out.W; ox++ {
			gx := want.X0 + ox
			ax := bilinearX(gx, srcW, dstW)
			x0 := clampInt(ax.i0, 0, srcW-1)
			x1 := clampInt(ax.i1, 0, srcW-1)

			var sr, sg, sb int64
			acc := func(gx2, gy2 int, wt int64) {
				sr += int64(t.get(gx2, gy2, 0)) * wt
				sg += int64(t.get(gx2, gy2, 1)) * wt
				sb += int64(t.get(gx2, gy2, 2)) * wt
			}
			acc(x0, y0, ax.w0*ay.w0)
			acc(x1, y0, ax.w1*ay.w0)
			acc(x0, y1, ax.w0*ay.w1)
			acc(x1, y1, ax.w1*ay.w1)

			idx := (oy*out.W + ox) * 3
			out.Pix[idx+0] = quantize(sr, denom)
			out.Pix[idx+1] = quantize(sg, denom)
			out.Pix[idx+2] = quantize(sb, denom)
		}
	}
	return out
}

package localrender

// applyBoxBlur produces the want rectangle of the image after applying a
// normalized box blur of radius r to the current image held in t.
//
// Each output pixel is the mean over the full (2r+1) x (2r+1) window centered
// on it; out-of-window samples repeat the nearest edge pixel, and the divisor
// is always the full window area. Sums are accumulated as exact integers with
// no intermediate rounding; quantization to 8 bits happens once per output
// pixel at the end.
func applyBoxBlur(t tile, r int, want Rect) tile {
	out := tile{GX: want.X0, GY: want.Y0, W: want.Width(), H: want.Height()}
	out.Pix = make([]uint8, out.W*out.H*3)
	side := int64(2*r + 1)
	denom := side * side
	for oy := 0; oy < out.H; oy++ {
		gy := want.Y0 + oy
		for ox := 0; ox < out.W; ox++ {
			gx := want.X0 + ox
			var sr, sg, sb int64
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					sr += int64(t.get(gx+dx, gy+dy, 0))
					sg += int64(t.get(gx+dx, gy+dy, 1))
					sb += int64(t.get(gx+dx, gy+dy, 2))
				}
			}
			idx := (oy*out.W + ox) * 3
			out.Pix[idx+0] = quantize(sr, denom)
			out.Pix[idx+1] = quantize(sg, denom)
			out.Pix[idx+2] = quantize(sb, denom)
		}
	}
	return out
}

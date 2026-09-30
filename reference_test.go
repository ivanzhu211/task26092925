package localrender

// fullFrame is the independent whole-image reference used by the tests. It
// always processes the entire image stage by stage and only crops at the end.
// Arithmetic mirrors the specification directly (exact integer weights and
// sums, edge clamping, single quantization per stage) but the code path is
// deliberately separate from the library implementation.
type fullFrame struct {
	w, h int
	pix  []uint8 // tightly packed RGB8
}

func newFullFrame(w, h int, pix []uint8) *fullFrame {
	return &fullFrame{w: w, h: h, pix: pix}
}

func (f *fullFrame) at(x, y, c int) uint8 {
	if x < 0 {
		x = 0
	} else if x >= f.w {
		x = f.w - 1
	}
	if y < 0 {
		y = 0
	} else if y >= f.h {
		y = f.h - 1
	}
	return f.pix[(y*f.w+x)*3+c]
}

func refRound(num, den int64) uint8 {
	v := (num + den/2) / den
	if v > 255 {
		v = 255
	}
	return uint8(v)
}

func (f *fullFrame) crop(op CropOp) {
	out := make([]uint8, op.W*op.H*3)
	for y := 0; y < op.H; y++ {
		for x := 0; x < op.W; x++ {
			for c := 0; c < 3; c++ {
				out[(y*op.W+x)*3+c] = f.pix[((y+op.Y)*f.w+(x+op.X))*3+c]
			}
		}
	}
	f.w, f.h, f.pix = op.W, op.H, out
}

func (f *fullFrame) resize(op ResizeOp) {
	oldW, oldH := f.w, f.h
	out := make([]uint8, op.W*op.H*3)
	denom := int64(2*op.W) * int64(2*op.H)
	for y := 0; y < op.H; y++ {
		ny := int64(2*y+1)*int64(oldH) - int64(op.H)
		nd := int64(2 * op.H)
		y0 := int(floorDiv(ny, nd))
		wy1 := ny - int64(y0)*nd
		wy0 := nd - wy1
		cy0 := clampInt(y0, 0, oldH-1)
		cy1 := clampInt(y0+1, 0, oldH-1)
		for x := 0; x < op.W; x++ {
			nx := int64(2*x+1)*int64(oldW) - int64(op.W)
			xd := int64(2 * op.W)
			x0 := int(floorDiv(nx, xd))
			wx1 := nx - int64(x0)*xd
			wx0 := xd - wx1
			cx0 := clampInt(x0, 0, oldW-1)
			cx1 := clampInt(x0+1, 0, oldW-1)
			for c := 0; c < 3; c++ {
				v := int64(f.pix[(cy0*oldW+cx0)*3+c])*wx0*wy0 +
					int64(f.pix[(cy0*oldW+cx1)*3+c])*wx1*wy0 +
					int64(f.pix[(cy1*oldW+cx0)*3+c])*wx0*wy1 +
					int64(f.pix[(cy1*oldW+cx1)*3+c])*wx1*wy1
				out[(y*op.W+x)*3+c] = refRound(v, denom)
			}
		}
	}
	f.w, f.h, f.pix = op.W, op.H, out
}

func (f *fullFrame) blur(op BoxBlurOp) {
	out := make([]uint8, f.w*f.h*3)
	r := op.R
	den := int64((2*r + 1) * (2*r + 1))
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			for c := 0; c < 3; c++ {
				var sum int64
				for dy := -r; dy <= r; dy++ {
					for dx := -r; dx <= r; dx++ {
						sum += int64(f.at(x+dx, y+dy, c))
					}
				}
				out[(y*f.w+x)*3+c] = refRound(sum, den)
			}
		}
	}
	f.pix = out
}

// runPlan processes the full frame through the whole plan.
func (f *fullFrame) runPlan(ops []Op) {
	for _, op := range ops {
		switch o := op.(type) {
		case CropOp:
			f.crop(o)
		case ResizeOp:
			f.resize(o)
		case BoxBlurOp:
			f.blur(o)
		}
	}
}

// sliceRect extracts rect from a full-frame result as tightly packed RGB8.
func (f *fullFrame) sliceRect(r Rect) []byte {
	out := make([]byte, r.Width()*r.Height()*3)
	for y := 0; y < r.Height(); y++ {
		for x := 0; x < r.Width(); x++ {
			for c := 0; c < 3; c++ {
				out[(y*r.Width()+x)*3+c] = f.pix[((r.Y0+y)*f.w+(r.X0+x))*3+c]
			}
		}
	}
	return out
}

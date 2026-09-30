package localrender

import (
	"io"
	"math"
	"math/rand"
	"testing"
)

// ---- Independent full-frame reference implementation ----
// It processes the whole image at every stage and is intentionally written
// independently from Render: dense []float64 frames, no dependency tracking.

type refFrame struct {
	w, h int
	pix  []float64 // w*h*3, channels innermost
}

func refNew(w, h int, src []byte) *refFrame {
	f := &refFrame{w: w, h: h, pix: make([]float64, w*h*3)}
	for i, b := range src {
		f.pix[i] = float64(b)
	}
	return f
}

func refQuant(v float64) uint8 {
	n := int(math.Floor(v + 0.5))
	if n < 0 {
		n = 0
	}
	if n > 255 {
		n = 255
	}
	return uint8(n)
}

func (f *refFrame) at(x, y, c int) float64 {
	if x < 0 {
		x = 0
	}
	if x >= f.w {
		x = f.w - 1
	}
	if y < 0 {
		y = 0
	}
	if y >= f.h {
		y = f.h - 1
	}
	return f.pix[(y*f.w+x)*3+c]
}

func refCrop(in *refFrame, op Op) *refFrame {
	out := &refFrame{w: op.w, h: op.h, pix: make([]float64, op.w*op.h*3)}
	for y := 0; y < op.h; y++ {
		for x := 0; x < op.w; x++ {
			for c := 0; c < 3; c++ {
				out.pix[(y*op.w+x)*3+c] = in.at(op.x+x, op.y+y, c)
			}
		}
	}
	return out
}

func refResize(in *refFrame, op Op) *refFrame {
	out := &refFrame{w: op.w, h: op.h, pix: make([]float64, op.w*op.h*3)}
	for y := 0; y < op.h; y++ {
		sy := (float64(y)+0.5)*float64(in.h)/float64(op.h) - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		for x := 0; x < op.w; x++ {
			sx := (float64(x)+0.5)*float64(in.w)/float64(op.w) - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			for c := 0; c < 3; c++ {
				v00 := in.at(x0, y0, c)
				v01 := in.at(x0+1, y0, c)
				v10 := in.at(x0, y0+1, c)
				v11 := in.at(x0+1, y0+1, c)
				top := v00*(1-fx) + v01*fx
				bot := v10*(1-fx) + v11*fx
				out.pix[(y*op.w+x)*3+c] = float64(refQuant(top*(1-fy) + bot*fy))
			}
		}
	}
	return out
}

func refBoxBlur(in *refFrame, op Op) *refFrame {
	d := 2*op.r + 1
	denom := float64(d * d)
	out := &refFrame{w: in.w, h: in.h, pix: make([]float64, in.w*in.h*3)}
	for y := 0; y < in.h; y++ {
		for x := 0; x < in.w; x++ {
			for c := 0; c < 3; c++ {
				sum := 0.0
				for ky := 0; ky < d; ky++ {
					for kx := 0; kx < d; kx++ {
						sum += in.at(x-op.r+kx, y-op.r+ky, c)
					}
				}
				out.pix[(y*in.w+x)*3+c] = float64(refQuant(sum / denom))
			}
		}
	}
	return out
}

func refRender(src []byte, w, h int, ops []Op) *refFrame {
	cur := refNew(w, h, src)
	for _, op := range ops {
		switch op.kind {
		case opCrop:
			cur = refCrop(cur, op)
		case opResize:
			cur = refResize(cur, op)
		case opBoxBlur:
			cur = refBoxBlur(cur, op)
		}
	}
	return cur
}

func refRect(f *refFrame, r Rect) []byte {
	out := make([]byte, r.W*r.H*3)
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			for c := 0; c < 3; c++ {
				out[(y*r.W+x)*3+c] = refQuant(f.pix[((r.Y+y)*f.w+(r.X+x))*3+c])
			}
		}
	}
	return out
}

// ---- helpers ----

type bytesReaderAt struct{ b []byte }

func (r bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func genPlan(rng *rand.Rand, w, h int) []Op {
	ops := []Op{}
	n := rng.Intn(6) // 0..5 ops, include no-op plans
	for i := 0; i < n; i++ {
		switch rng.Intn(3) {
		case 0:
			cw := 1 + rng.Intn(w)
			ch := 1 + rng.Intn(h)
			cx := rng.Intn(w - cw + 1)
			cy := rng.Intn(h - ch + 1)
			ops = append(ops, Crop(cx, cy, cw, ch))
			w, h = cw, ch
		case 1:
			nw := 1 + rng.Intn(2*w) // 1..2w, includes non-integer ratios
			nh := 1 + rng.Intn(2*h)
			ops = append(ops, Resize(nw, nh))
			w, h = nw, nh
		case 2:
			r := rng.Intn(3) // 0..2, allows repeated/consecutive blur
			ops = append(ops, BoxBlur(r))
		}
	}
	return ops
}

// ---- tests ----

func TestRandomPlansByteExact(t *testing.T) {
	for seed := int64(1); seed <= 300; seed++ {
		rng := rand.New(rand.NewSource(seed))
		w := 1 + rng.Intn(7)
		h := 1 + rng.Intn(7)
		src := make([]byte, w*h*3)
		rng.Read(src)
		ops := genPlan(rng, w, h)

		fw, fh := w, h
		for _, op := range ops {
			if op.kind == opCrop || op.kind == opResize {
				fw, fh = op.w, op.h
			}
		}
		ref := refRender(src, w, h, ops)

		// random rects, including edge rects and single-pixel rects
		nr := 1 + rng.Intn(4)
		for k := 0; k < nr; k++ {
			rw := 1 + rng.Intn(fw)
			rh := 1 + rng.Intn(fh)
			rx := rng.Intn(fw - rw + 1)
			ry := rng.Intn(fh - rh + 1)
			rect := Rect{rx, ry, rw, rh}
			got := Render(bytesReaderAt{src}, w, h, ops, rect)
			want := refRect(ref, rect)
			if string(got) != string(want) {
				t.Fatalf("seed=%d w=%d h=%d ops=%+v rect=%+v\n got=%v\nwant=%v",
					seed, w, h, ops, rect, got, want)
			}
		}

		// always check the full final frame too
		full := Rect{0, 0, fw, fh}
		if got, want := Render(bytesReaderAt{src}, w, h, ops, full), refRect(ref, full); string(got) != string(want) {
			t.Fatalf("seed=%d full mismatch ops=%+v", seed, ops)
		}
	}
}

func TestHalfIntegerRounding(t *testing.T) {
	// 2x2 -> 1x1 resize: both source weights are exactly 0.5.
	// Values chosen so the channel average is an exact half-integer (x.5):
	// (0+0+0+1)/4 = 0.25 bilinear, so use channel patterns instead.
	src := []byte{
		0, 1, 2, 1, 2, 3,
		2, 3, 4, 3, 4, 5,
	}
	ops := []Op{Resize(1, 1)}
	got := Render(bytesReaderAt{src}, 2, 2, ops, Rect{0, 0, 1, 1})
	// channel means: 1.5, 2.5, 3.5 -> all halves round up to 2,3,4
	want := []byte{2, 3, 4}
	if string(got) != string(want) {
		t.Fatalf("half rounding got=%v want=%v", got, want)
	}

	// 1x2 -> 1x1 vertical-only average 0 and 1 => 0.5 rounds up to 1.
	src2 := []byte{0, 10, 20, 1, 11, 21}
	got2 := Render(bytesReaderAt{src2}, 1, 2, []Op{Resize(1, 1)}, Rect{0, 0, 1, 1})
	want2 := []byte{1, 11, 21} // 0.5,10.5,20.5
	if string(got2) != string(want2) {
		t.Fatalf("vertical half rounding got=%v want=%v", got2, want2)
	}
}

func TestConsecutiveBlurAndCropEdges(t *testing.T) {
	rng := rand.New(rand.NewSource(424242))
	w, h := 6, 5
	src := make([]byte, w*h*3)
	rng.Read(src)
	ops := []Op{
		BoxBlur(1), BoxBlur(1),
		Crop(1, 1, 4, 3),
		BoxBlur(2),
		Resize(7, 4),
		BoxBlur(1),
		Resize(3, 6),
	}
	ref := refRender(src, w, h, ops)
	for _, rect := range []Rect{
		{0, 0, 3, 6},
		{0, 0, 1, 1},
		{2, 5, 1, 1},
		{1, 2, 2, 2},
	} {
		if got, want := Render(bytesReaderAt{src}, w, h, ops, rect), refRect(ref, rect); string(got) != string(want) {
			t.Fatalf("rect=%+v got=%v want=%v", rect, got, want)
		}
	}
}

func TestSourceNotMutated(t *testing.T) {
	src := []byte{
		10, 20, 30, 40, 50, 60,
		70, 80, 90, 100, 110, 120,
	}
	backup := append([]byte(nil), src...)
	ops := []Op{BoxBlur(1), Resize(3, 3)}
	Render(bytesReaderAt{src}, 2, 2, ops, Rect{0, 0, 3, 3})
	if string(src) != string(backup) {
		t.Fatalf("source mutated")
	}
}

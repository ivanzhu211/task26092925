package localrender

import (
	"io"
	"math"
)

// Rect is an axis-aligned rectangle relative to the final image.
type Rect struct {
	X, Y, W, H int
}

// half-open integer rectangle in the coordinate space of one image stage.
type bounds struct {
	x0, y0, x1, y1 int
}

// frame is a tightly packed sub-rectangle of a logical full image.
type frame struct {
	data   []byte
	ox, oy int // logical coordinate of the first pixel in data
	w, h   int // sub-rectangle size
	W, H   int // full logical image size
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

// quant rounds to the nearest 8-bit value; exact halves round up.
func quant(v float64) uint8 {
	n := int(math.Round(v))
	return uint8(clampInt(n, 0, 255))
}

func (f frame) channel(x, y, c int) float64 {
	x = clampInt(x, 0, f.W-1)
	y = clampInt(y, 0, f.H-1)
	i := ((y-f.oy)*f.w+(x-f.ox))*3 + c
	return float64(f.data[i])
}

// stageSizes returns the full image size before every op and after the last.
func stageSizes(width, height int, ops []Op) []struct{ w, h int } {
	sizes := make([]struct{ w, h int }, len(ops)+1)
	sizes[0].w = width
	sizes[0].h = height
	w, h := width, height
	for i, op := range ops {
		switch op.kind {
		case opCrop:
			w, h = op.w, op.h
		case opResize:
			w, h = op.w, op.h
		case opBoxBlur:
			// size unchanged
		}
		sizes[i+1].w = w
		sizes[i+1].h = h
	}
	return sizes
}

// inputBounds returns the pixels of an op's input image that can affect out.
func inputBounds(op Op, inW, inH int, out bounds) bounds {
	switch op.kind {
	case opCrop:
		return bounds{
			x0: out.x0 + op.x,
			y0: out.y0 + op.y,
			x1: out.x1 + op.x,
			y1: out.y1 + op.y,
		}
	case opResize:
		mapX := func(x int) float64 {
			return (float64(x)+0.5)*float64(inW)/float64(op.w) - 0.5
		}
		mapY := func(y int) float64 {
			return (float64(y)+0.5)*float64(inH)/float64(op.h) - 0.5
		}
		left := clampInt(int(math.Floor(mapX(out.x0))), 0, inW-1)
		right := clampInt(int(math.Floor(mapX(out.x1-1)))+1, 0, inW-1) + 1
		top := clampInt(int(math.Floor(mapY(out.y0))), 0, inH-1)
		bottom := clampInt(int(math.Floor(mapY(out.y1-1)))+1, 0, inH-1) + 1
		return bounds{x0: left, y0: top, x1: right, y1: bottom}
	case opBoxBlur:
		return bounds{
			x0: clampInt(out.x0-op.r, 0, inW),
			y0: clampInt(out.y0-op.r, 0, inH),
			x1: clampInt(out.x1+op.r, 0, inW),
			y1: clampInt(out.y1+op.r, 0, inH),
		}
	}
	return out
}

func readRegion(reader io.ReaderAt, srcW, srcH int, b bounds) frame {
	buf := make([]byte, b.w()*b.h()*3)
	rowBytes := b.w() * 3
	for y := b.y0; y < b.y1; y++ {
		off := int64(y*srcW+b.x0) * 3
		dst := buf[(y-b.y0)*rowBytes:]
		n, err := reader.ReadAt(dst[:rowBytes], off)
		if err != nil || n != rowBytes {
			panic("localrender: source read failed")
		}
	}
	return frame{data: buf, ox: b.x0, oy: b.y0, w: b.w(), h: b.h(), W: srcW, H: srcH}
}

func (b bounds) w() int { return b.x1 - b.x0 }
func (b bounds) h() int { return b.y1 - b.y0 }

func applyCrop(in frame, op Op) frame {
	return frame{
		data: in.data,
		ox:   in.ox - op.x,
		oy:   in.oy - op.y,
		w:    in.w,
		h:    in.h,
		W:    op.w,
		H:    op.h,
	}
}

func applyResize(in frame, op Op, out bounds) frame {
	outF := frame{
		data: make([]byte, out.w()*out.h()*3),
		ox:   out.x0,
		oy:   out.y0,
		w:    out.w(),
		h:    out.h(),
		W:    op.w,
		H:    op.h,
	}
	for y := out.y0; y < out.y1; y++ {
		sy := (float64(y)+0.5)*float64(in.H)/float64(op.h) - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		for x := out.x0; x < out.x1; x++ {
			sx := (float64(x)+0.5)*float64(in.W)/float64(op.w) - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			x1 := x0 + 1
			y1 := y0 + 1
			ci := ((y-out.y0)*out.w() + (x - out.x0)) * 3
			for c := 0; c < 3; c++ {
				v00 := in.channel(x0, y0, c)
				v01 := in.channel(x1, y0, c)
				v10 := in.channel(x0, y1, c)
				v11 := in.channel(x1, y1, c)
				top := (1-fx)*v00 + fx*v01
				bot := (1-fx)*v10 + fx*v11
				v := (1-fy)*top + fy*bot
				outF.data[ci+c] = quant(v)
			}
		}
	}
	return outF
}

func applyBoxBlur(in frame, op Op, out bounds) frame {
	outF := frame{
		data: make([]byte, out.w()*out.h()*3),
		ox:   out.x0,
		oy:   out.y0,
		w:    out.w(),
		h:    out.h(),
		W:    in.W,
		H:    in.H,
	}
	d := 2*op.r + 1
	denom := float64(d * d)
	for y := out.y0; y < out.y1; y++ {
		for x := out.x0; x < out.x1; x++ {
			ci := ((y-out.y0)*out.w() + (x - out.x0)) * 3
			for c := 0; c < 3; c++ {
				var sum float64
				for ky := -op.r; ky <= op.r; ky++ {
					for kx := -op.r; kx <= op.r; kx++ {
						sum += in.channel(x+kx, y+ky, c)
					}
				}
				outF.data[ci+c] = quant(sum / denom)
			}
		}
	}
	return outF
}

// Render returns the RGB8 bytes of rect from the final image produced by ops.
// The source image is width*height tightly packed RGB8 rows read from reader.
func Render(reader io.ReaderAt, width, height int, ops []Op, rect Rect) []byte {
	sizes := stageSizes(width, height, ops)

	needed := make([]bounds, len(ops)+1)
	needed[len(ops)] = bounds{
		x0: rect.X,
		y0: rect.Y,
		x1: rect.X + rect.W,
		y1: rect.Y + rect.H,
	}
	for i := len(ops) - 1; i >= 0; i-- {
		needed[i] = inputBounds(ops[i], sizes[i].w, sizes[i].h, needed[i+1])
	}

	cur := readRegion(reader, width, height, needed[0])
	for i, op := range ops {
		switch op.kind {
		case opCrop:
			cur = applyCrop(cur, op)
		case opResize:
			cur = applyResize(cur, op, needed[i+1])
		case opBoxBlur:
			cur = applyBoxBlur(cur, op, needed[i+1])
		}
	}

	result := make([]byte, rect.W*rect.H*3)
	rowBytes := rect.W * 3
	for y := 0; y < rect.H; y++ {
		srcRow := (y+rect.Y-cur.oy)*cur.w + (rect.X - cur.ox)
		copy(result[y*rowBytes:(y+1)*rowBytes], cur.data[srcRow*3:srcRow*3+rowBytes])
	}
	return result
}

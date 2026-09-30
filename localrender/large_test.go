package localrender

import (
	"io"
	"runtime"
	"testing"
)

// proceduralReader serves a deterministic huge RGB8 image without storing it.
type proceduralReader struct {
	w, h     int
	calls    int
	bytes    int64
	firstRow int64
	lastRow  int64
	seenRows map[int64]struct{}
}

func newProceduralReader(w, h int) *proceduralReader {
	return &proceduralReader{w: w, h: h, firstRow: -1, seenRows: map[int64]struct{}{}}
}

func (p *proceduralReader) pixelByte(off int64) byte {
	pos := off / 3
	y := pos / int64(p.w)
	x := pos % int64(p.w)
	c := off % 3
	v := x*73856093 ^ y*19349663 ^ int64(c)*83492791
	return byte(v % 251)
}

func (p *proceduralReader) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 || off > int64(p.w*p.h*3) {
		return 0, io.EOF
	}
	n := len(b)
	if off+int64(n) > int64(p.w*p.h*3) {
		n = int(int64(p.w*p.h*3) - off)
	}
	p.calls++
	p.bytes += int64(n)
	rowStride := int64(p.w * 3)
	row := off / rowStride
	endRow := (off + int64(n) - 1) / rowStride
	if p.firstRow < 0 || row < p.firstRow {
		p.firstRow = row
	}
	if endRow > p.lastRow {
		p.lastRow = endRow
	}
	for r := row; r <= endRow; r++ {
		p.seenRows[r] = struct{}{}
	}
	for i := 0; i < n; i++ {
		b[i] = p.pixelByte(off + int64(i))
	}
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func TestLargeLocalReadAndMemory(t *testing.T) {
	const (
		W, H   = 20000, 20000
		nw, nh = 200, 200
		rw, rh = 10, 10
	)
	totalBytes := int64(W) * int64(H) * 3

	ops := []Op{BoxBlur(1), BoxBlur(1), Resize(nw, nh)}
	rect := Rect{(nw - rw) / 2, (nh - rh) / 2, rw, rh}

	r := newProceduralReader(W, H)
	got := Render(r, W, H, ops, rect)
	if len(got) != rw*rh*3 {
		t.Fatalf("unexpected output size %d", len(got))
	}

	// Repeatability: deterministic output across renders.
	r2 := newProceduralReader(W, H)
	got2 := Render(r2, W, H, ops, rect)
	if string(got) != string(got2) {
		t.Fatal("render not deterministic")
	}

	// Local reads: only a compact block of rows near the center is touched.
	// The dependency spans ~906 rows (~2.6MB), nowhere near the 1.2GB frame.
	if int64(r.bytes) >= totalBytes/100 {
		t.Fatalf("read %d bytes, expected < %d (1%% of source)", r.bytes, totalBytes/100)
	}
	if len(r.seenRows) != int(r.lastRow-r.firstRow+1) {
		t.Fatalf("non-contiguous rows read: %d rows over span %d", len(r.seenRows), r.lastRow-r.firstRow+1)
	}
	centerRow := int64(H / 2)
	if r.firstRow < centerRow-2000 || r.lastRow > centerRow+2000 {
		t.Fatalf("read rows %d..%d not local to center %d", r.firstRow, r.lastRow, centerRow)
	}
	t.Logf("local reads: %d ReadAt calls, %d bytes (%d contiguous rows)",
		r.calls, r.bytes, len(r.seenRows))

	// Repeatable memory check: bytes allocated per Render must stay small and
	// independent of the full 400-megapixel frame.
	const runs = 5
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	rr := newProceduralReader(W, H)
	for i := 0; i < runs; i++ {
		out := Render(rr, W, H, ops, rect)
		if string(out) != string(got) {
			t.Fatal("output changed across memory-check runs")
		}
	}
	runtime.ReadMemStats(&m1)
	perRun := float64(m1.TotalAlloc-m0.TotalAlloc) / runs
	const budget = 20 << 20
	if perRun >= budget {
		t.Fatalf("allocated %.1f bytes/run, budget %d", perRun, budget)
	}
	t.Logf("allocation: %.1f KiB/run (budget %d MiB)", perRun/1024, budget>>20)
}

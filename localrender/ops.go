package localrender

// OpKind identifies the kind of an edit operation.
type OpKind int

const (
	opCrop OpKind = iota + 1
	opResize
	opBoxBlur
)

// Op is a single edit in an ordered edit plan.
type Op struct {
	kind OpKind
	x, y int
	w, h int
	r    int
}

// Crop keeps w columns and h rows starting at integer coordinate (x, y).
func Crop(x, y, w, h int) Op {
	return Op{kind: opCrop, x: x, y: y, w: w, h: h}
}

// Resize resamples the current image to (w, h) with bilinear interpolation.
func Resize(w, h int) Op {
	return Op{kind: opResize, w: w, h: h}
}

// BoxBlur applies a (2r+1)x(2r+1) box blur with edge replication.
func BoxBlur(r int) Op {
	return Op{kind: opBoxBlur, r: r}
}

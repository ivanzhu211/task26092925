package localrender

import "io"

// readTile reads rect from a source image of width srcW stored as tightly
// packed row-major RGB8 bytes. Each source row is fetched with one ReadAt
// call; the returned tile is itself tightly packed over rect's dimensions.
// The reader is only ever queried within rect, and the source is never
// written.
func readTile(reader io.ReaderAt, srcW int, rect Rect) tile {
	t := tile{
		GX: rect.X0,
		GY: rect.Y0,
		W:  rect.Width(),
		H:  rect.Height(),
	}
	t.Pix = make([]uint8, t.W*t.H*3)
	rowBytes := t.W * 3
	for row := 0; row < t.H; row++ {
		offset := int64((rect.Y0+row)*srcW+rect.X0) * 3
		dst := t.Pix[row*rowBytes : (row+1)*rowBytes]
		if _, err := reader.ReadAt(dst, offset); err != nil {
			// Inputs are guaranteed valid and fully in range; an unexpected
			// short read indicates a broken source.
			panic("localrender: source read failed: " + err.Error())
		}
	}
	return t
}

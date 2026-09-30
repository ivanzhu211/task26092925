package localrender

// tile is a rectangular window of an intermediate (or source) image. Pix is
// tightly packed RGB8; global pixel (gx, gy) is stored at
// (gy-GY)*W*3 + (gx-GX)*3 + channel.
type tile struct {
	GX, GY int
	W, H   int
	Pix    []uint8
}

// get returns the value of channel c of the global pixel (gx, gy). Samples
// outside the tile's window are clamped to its nearest edge, so get behaves
// exactly like edge-clamped sampling of the full image as long as the tile
// covers every unclamped neighbor of every rendered pixel.
func (t tile) get(gx, gy, c int) uint8 {
	x := clampInt(gx-t.GX, 0, t.W-1)
	y := clampInt(gy-t.GY, 0, t.H-1)
	return t.Pix[(y*t.W+x)*3+c]
}

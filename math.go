package localrender

// quantize evaluates num/denom exactly and rounds to the nearest integer,
// breaking an exact half-integer tie upward. num must be non-negative and
// denom positive. The mathematical value never exceeds 255 for the kernels
// here; the result is clamped defensively.
func quantize(num, denom int64) uint8 {
	q := (num + denom/2) / denom
	if q > 255 {
		return 255
	}
	return uint8(q)
}

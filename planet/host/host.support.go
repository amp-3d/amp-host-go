package host


type Range struct {
	L       uint64
	R       uint64
}

// UnionRange unions the given range (inclusive) with current range 
func (r *Range) UnionRange(L, R uint64) {
	if L < r.L {
		r.L = L
	}
	if R > r.R {
		r.R = R
	}
}

package shuffle

// Shuffle cryptographic hash based Fisher–Yates shuffle algorithm.
// The perm is to receive shuffled permutation of [0, len(perm)-1).
func Shuffle(seed []byte, perm []int) {
	for i := range perm {
		perm[i] = i
	}
	size := len(perm)
	if size < 2 {
		return
	}
	hr := newHrand(seed)
	for i := 0; i < len(perm)-1; i++ {
		j := hr.Intn(size-i) + i
		perm[i], perm[j] = perm[j], perm[i]
	}
}

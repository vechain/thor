package shuffle

// Shuffle cryptographic hash based Fisher–Yates shuffle algorithm.
// It generates a shuffled slice based on seed.
// The size indicates the length of returned slice.
func Shuffle(seed uint32, size int) []int {
	slice := make([]int, size)
	for i := range slice {
		slice[i] = i
	}

	hr := newHrand(seed)
	for i := 0; i < size-1; i++ {
		j := hr.Intn(size-i) + i

		tmp := slice[i]
		slice[i] = slice[j]
		slice[j] = tmp
	}
	return slice
}

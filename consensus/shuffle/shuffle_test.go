package shuffle_test

import (
	"fmt"
	"testing"

	"github.com/vechain/thor/consensus/shuffle"
)

func TestShuffle(t *testing.T) {
	sum := make([]int, 10)
	perm := make([]int, 10)

	for i := uint32(0); i < 10000; i++ {
		shuffle.Shuffle(i, perm)
		for j := range sum {
			sum[j] = sum[j] + perm[j]
		}
	}
	fmt.Println(sum)
}

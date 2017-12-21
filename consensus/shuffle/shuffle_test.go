package shuffle_test

import (
	"fmt"
	"testing"

	"github.com/vechain/thor/consensus/shuffle"
)

func TestShuffle(t *testing.T) {
	sum := make([]int, 10)
	for i := uint32(0); i < 10000; i++ {
		s := shuffle.Shuffle(i, 10)
		for j := range sum {
			sum[j] = sum[j] + s[j]
		}
	}
	fmt.Println(sum)
}

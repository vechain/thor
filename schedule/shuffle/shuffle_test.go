package shuffle_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/vechain/thor/schedule/shuffle"
)

func TestShuffle(t *testing.T) {
	sum := make([]int, 10)
	perm := make([]int, 10)

	var seed [4]byte
	for i := uint32(0); i < 100; i++ {
		binary.BigEndian.PutUint32(seed[:], i)
		shuffle.Shuffle(seed[:], perm)
		for j := range sum {
			sum[j] = sum[j] + perm[j]
		}
	}
	fmt.Println(sum)
}

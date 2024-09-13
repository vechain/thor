package datagen

import (
	"crypto/rand"
	"github.com/vechain/thor/v2/thor"
)

func RandBytes32() (b thor.Bytes32) {
	rand.Read(b[:])
	return
}

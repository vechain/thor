package cry

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

var emptyHash = common.FromHex("0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470")
var valueHash = common.FromHex("0x22ae6da6b482f9b1b19b0b897c3fd43884180a1c5ee361e1107a1bc635649dda")

func TestHash(t *testing.T) {

	v := VSha3([]byte{})
	if !bytes.Equal(emptyHash, v) {
		t.Fatalf("empty hash mismatch: want: %x have: %x", emptyHash, v)
	}
	v = VSha3([]byte{1, 2})
	if !bytes.Equal(valueHash, v) {
		t.Fatalf("empty hash mismatch: want: %x have: %x", valueHash, v)
	}
}

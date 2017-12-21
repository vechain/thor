package shuffle

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

// hash based random generator
type hrand struct {
	sha  hash.Hash
	seed [32]byte
	seq  uint32
}

func newHrand(seed uint32) *hrand {
	var hr hrand
	hr.sha = sha256.New()
	binary.BigEndian.PutUint32(hr.seed[:], seed)
	return &hr
}

// returns int in [0, n)
// panic if n <=0
func (hr *hrand) Intn(n int) int {
	if n <= 0 {
		panic("n must > 0")
	}
	p := hr.seq % 8
	hr.seq++
	if p == 0 {
		hr.sha.Write(hr.seed[:])
		hr.sha.Sum(hr.seed[:0])
	}
	return int(binary.BigEndian.Uint32(hr.seed[p*4:]) % uint32(n))
}

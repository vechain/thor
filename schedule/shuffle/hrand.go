package shuffle

import (
	"crypto/sha256"
	"encoding/binary"
)

// hash based random generator
type hrand struct {
	seed  []byte
	round uint32
	hash  [32]byte
	seq   uint32
}

func newHrand(seed []byte) *hrand {
	hseed := make([]byte, len(seed)+4)
	copy(hseed[4:], seed)
	return &hrand{seed: hseed}
}

func (hr *hrand) nextRound() {
	binary.BigEndian.PutUint32(hr.seed, hr.round)
	hr.hash = sha256.Sum256(hr.seed)
	hr.round++
}

// returns int in [0, n)
// panic if n <=0
func (hr *hrand) Intn(n int) int {
	if n <= 0 {
		panic("n must > 0")
	}
	i := hr.seq % 8
	if i == 0 {
		hr.nextRound()
	}
	hr.seq++
	return int(binary.BigEndian.Uint32(hr.hash[i*4:]) % uint32(n))
}

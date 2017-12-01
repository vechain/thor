package cry

import (
	"github.com/ethereum/go-ethereum/common"
)

/////////// Address to bytes
func AddressToBytes(addr common.Address) []byte {
	a := make([]byte, 0)
	a = append(a, addr[:]...)
	return a
}

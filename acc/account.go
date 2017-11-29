package acc

import (
	"math/big"

	"github.com/vechain/vecore/cry"
)

type Account struct {
	Balance     *big.Int
	CodeHash    cry.Hash
	StorageRoot cry.Hash // merkle root of the storage trie
}

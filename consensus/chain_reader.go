package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain/persist"
	"github.com/vechain/thor/thor"
)

type chainReader interface {
	IsNotFound(error) bool
	GetBlockHeader(thor.Hash) (*block.Header, error)
	LookupTransaction(thor.Hash, thor.Hash) (*persist.TxLocation, error)
}

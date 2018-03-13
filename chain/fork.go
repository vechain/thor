package chain

import (
	"github.com/vechain/thor/block"
)

// Fork describes forked chain.
type Fork struct {
	Ancestor *block.Block
	Trunk    []*block.Block
	Branch   []*block.Block
}

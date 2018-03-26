package poa

import (
	"github.com/vechain/thor/thor"
)

// Proposer address with status.
type Proposer struct {
	Address thor.Address
	Active  bool
}

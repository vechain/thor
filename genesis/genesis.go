package genesis

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/genesis/builder"
	cs "github.com/vechain/thor/genesis/contracts"
	"github.com/vechain/thor/state"
)

var (
	// GodAddress is the address that can do special things on genesis contracts.
	// e.g. initialize genesis contracts, consume/reward energy.
	GodAddress = acc.BytesToAddress([]byte("god"))

	// InitialGasLimit gas limit value int genesis block.
	InitialGasLimit uint64 = 10 * 1000 * 1000
)

const (
	// Timestamp timestamp of genesis block.
	Timestamp uint64 = 1234567890
)

// Build build the genesis block.
func Build(state *state.State) (*block.Block, error) {
	return new(builder.Builder).
		Timestamp(Timestamp).
		GasLimit(InitialGasLimit).
		Alloc(
			cs.Authority.Address,
			new(big.Int),
			cs.Authority.RuntimeBytecodes(),
		).
		Alloc(
			cs.Energy.Address,
			new(big.Int),
			cs.Energy.RuntimeBytecodes(),
		).
		Call(
			cs.Authority.Address,
			func() []byte {
				// Authority.initialize(address, address[])
				data, err := cs.Authority.ABI.Pack(
					"initialize",
					acc.BytesToAddress([]byte("test")),
					[]acc.Address{})
				if err != nil {
					panic(errors.Wrap(err, "build genesis"))
				}
				return data
			}(),
		).
		Build(state, GodAddress)
}

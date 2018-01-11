package genesis

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/genesis/builder"
	cs "github.com/vechain/thor/genesis/contracts"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

const (
	// Timestamp timestamp of genesis block.
	Timestamp uint64 = 1234567890
)

// Build build the genesis block.
func Build(state *state.State) (*block.Block, error) {
	return new(builder.Builder).
		Timestamp(Timestamp).
		GasLimit(thor.InitialGasLimit).
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
					thor.BytesToAddress([]byte("test")),
					[]thor.Address{})
				if err != nil {
					panic(errors.Wrap(err, "build genesis"))
				}
				return data
			}(),
		).
		Build(state)
}

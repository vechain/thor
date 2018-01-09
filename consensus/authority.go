package consensus

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/consensus/schedule"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/genesis/contracts"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

func PredicateTrunk(state *state.State, header *block.Header, preHeader *block.Header) (bool, error) {
	signer, err := header.Signer()
	if err != nil {
		return false, err
	}

	rt := runtime.New(state, preHeader, nil, vm.Config{})

	return schedule.New(
		Authority(rt, "getProposers"),
		Authority(rt, "getAbsentee"),
		preHeader.Number(),
		preHeader.Timestamp()).Validate(*signer, header.Timestamp())
}

func Authority(rt *runtime.Runtime, funcName string) []acc.Address {
	clause := &tx.Clause{
		To: &contracts.Authority.Address,
		Data: func() []byte {
			data, err := contracts.Authority.ABI.Pack(
				funcName)
			if err != nil {
				panic(errors.Wrap(err, fmt.Sprintf("call %s\n", funcName)))
			}
			return data
		}()}

	output := rt.Exec(clause, 0, math.MaxUint64, genesis.GodAddress, new(big.Int), cry.Hash{})
	var addrs []common.Address
	if err := contracts.Authority.ABI.Unpack(&addrs, funcName, output.Value); err != nil {
		panic(err)
	}

	return convertToAccAddress(addrs)
}

func convertToAccAddress(addrs []common.Address) []acc.Address {
	length := len(addrs)
	if length == 0 {
		return nil
	}
	return append(convertToAccAddress(addrs[1:length]), acc.Address(addrs[0]))
}

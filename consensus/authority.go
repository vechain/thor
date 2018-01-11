package consensus

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/consensus/schedule"
	"github.com/vechain/thor/genesis/contracts"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func PredicateTrunk(state *state.State, header *block.Header, preHeader *block.Header) (bool, error) {
	signer, err := header.Signer()
	if err != nil {
		return false, err
	}

	rt := runtime.New(state, preHeader, nil)

	return schedule.New(
		Authority(rt, "getProposers"),
		Authority(rt, "getAbsentee"),
		preHeader.Number(),
		preHeader.Timestamp()).Validate(signer, header.Timestamp())
}

func Authority(rt *runtime.Runtime, funcName string) []thor.Address {
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

	output := rt.Execute(clause, 0, math.MaxUint64, thor.Address{}, &big.Int{}, thor.Hash{})
	var addrs []common.Address
	if err := contracts.Authority.ABI.Unpack(&addrs, funcName, output.Value); err != nil {
		panic(err)
	}

	return convertToAccAddress(addrs)
}

func convertToAccAddress(addrs []common.Address) []thor.Address {
	length := len(addrs)
	if length == 0 {
		return nil
	}
	return append(convertToAccAddress(addrs[1:length]), thor.Address(addrs[0]))
}

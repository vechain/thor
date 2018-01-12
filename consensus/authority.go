package consensus

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Authority can call getProposers & getAbsentee methods of Authority.sol.
func Authority(rt *runtime.Runtime, funcName string) []thor.Address {
	clause := &tx.Clause{
		To: &contracts.Authority.Address,
		Data: func() []byte {
			data, err := contracts.Authority.ABI.Pack(funcName)
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

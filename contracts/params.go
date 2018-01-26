package contracts

import (
	"math/big"

	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// keys of governance params.
const (
	ParamRewardRatio  = "reward-ratio" // use 1e18 as denominator
	ParamBaseGasPrice = "base-gas-price"
)

// Params binder of `Params` contract.
var Params = func() params {
	addr := thor.BytesToAddress([]byte("par"))
	return params{
		addr,
		mustLoad("compiled/Params.abi", "compiled/Params.bin-runtime"),
		tx.NewClause(&addr),
	}
}()

type params struct {
	Address thor.Address
	contract
	clause *tx.Clause
}

// PackInitialize packs input data of `Params.sysInitialize` function.
func (p *params) PackInitialize(voting thor.Address) *tx.Clause {
	return p.clause.WithData(p.mustPack("sysInitialize", voting))
}

// PackPreset packs input data of `Params.sysPreset` function.
func (p *params) PackPreset(key string, value *big.Int) *tx.Clause {
	return p.clause.WithData(p.mustPack("sysPreset", key, value))
}

// PackGet packs input data of `Params.get` function.
func (p *params) PackGet(key string) *tx.Clause {
	return p.clause.WithData(p.mustPack("get", key))
}

// UnpackGet unpacks output data of `Params.get` function.
func (p *params) UnpackGet(output []byte) *big.Int {
	var value *big.Int
	p.mustUnpack(&value, "get", output)
	return value
}

// PackSet packs input data of `Params.set` function.
func (p *params) PackSet(key string, value *big.Int) *tx.Clause {
	return p.clause.WithData(p.mustPack("set", key, value))
}

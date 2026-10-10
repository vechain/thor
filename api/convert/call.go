// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

// CallGas resolves the gas of a simulated call: zero means limit, and more
// than limit is rejected with 403.
func CallGas(gas, limit uint64) (uint64, error) {
	if gas > limit {
		return 0, restutil.Forbidden(errors.New("gas: exceeds limit"))
	} else if gas == 0 {
		return limit, nil
	}
	return gas, nil
}

// BuildCallTxContext builds the transaction context of a simulated call.
// Nil prices and addresses default to zero; a malformed blockRef is a 400.
func BuildCallTxContext(
	clauseCount uint32,
	gasPrice, provedWork *math.HexOrDecimal256,
	caller, gasPayer *thor.Address,
	expiration uint32,
	blockRef string,
) (*xenv.TransactionContext, error) {
	txCtx := &xenv.TransactionContext{
		ClauseCount: clauseCount,
		Expiration:  expiration,
	}
	if gasPrice == nil {
		txCtx.GasPrice = new(big.Int)
	} else {
		txCtx.GasPrice = (*big.Int)(gasPrice)
	}
	if caller == nil {
		txCtx.Origin = thor.Address{}
	} else {
		txCtx.Origin = *caller
	}
	if gasPayer == nil {
		txCtx.GasPayer = thor.Address{}
	} else {
		txCtx.GasPayer = *gasPayer
	}
	if provedWork == nil {
		txCtx.ProvedWork = new(big.Int)
	} else {
		txCtx.ProvedWork = (*big.Int)(provedWork)
	}

	if len(blockRef) > 0 {
		blkRef, err := restutil.ParseBlockRef(blockRef)
		if err != nil {
			return nil, err
		}
		txCtx.BlockRef = blkRef
	}
	return txCtx, nil
}

// BuildBlockContext builds the block context for running a call on top of header.
func BuildBlockContext(header *block.Header) *xenv.BlockContext {
	signer, _ := header.Signer()
	return &xenv.BlockContext{
		Beneficiary: header.Beneficiary(),
		Signer:      signer,
		Number:      header.Number(),
		Time:        header.Timestamp(),
		GasLimit:    header.GasLimit(),
		TotalScore:  header.TotalScore(),
		BaseFee:     header.BaseFee(),
	}
}

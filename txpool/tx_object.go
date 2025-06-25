// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type txObject struct {
	*tx.Transaction
	resolved *runtime.ResolvedTransaction

	timeAdded      int64
	localSubmitted bool          // tx is submitted locally on this node, or synced remotely from p2p.
	payer          *thor.Address // payer of the tx, either origin, delegator, or on-chain delegation payer
	cost           *big.Int      // total tx cost the payer needs to pay before execution(gas price * gas)

	executable       bool     // don't touch this value, will be updated by the pool
	priorityGasPrice *big.Int // don't touch this value, it's only be used in pool's housekeeping
}

func resolveTx(tx *tx.Transaction, localSubmitted bool) (*txObject, error) {
	resolved, err := runtime.ResolveTransaction(tx)
	if err != nil {
		return nil, err
	}

	return &txObject{
		Transaction:    tx,
		resolved:       resolved,
		timeAdded:      time.Now().UnixNano(),
		localSubmitted: localSubmitted,
	}, nil
}

func (o *txObject) Origin() thor.Address {
	return o.resolved.Origin
}

func (o *txObject) Delegator() *thor.Address {
	return o.resolved.Delegator
}

func (o *txObject) Cost() *big.Int {
	return o.cost
}

func (o *txObject) Payer() *thor.Address {
	return o.payer
}

func (o *txObject) Executable(chain *chain.Chain, state *state.State, headBlock *block.Header, forkConfig *thor.ForkConfig) (bool, error) {
	// evaluate the tx on the next block as head block is already history
	nextBlockNum := headBlock.Number() + 1
	nextBlockTime := headBlock.Timestamp() + thor.BlockInterval

	switch {
	case o.Gas() > headBlock.GasLimit():
		return false, errors.New("gas too large")
	case o.IsExpired(nextBlockNum): // Check tx expiration on top of next block
		return false, errors.New("expired")
	case o.BlockRef().Number() > nextBlockNum+uint32(5*60/thor.BlockInterval):
		// reject deferred tx which will be applied after 5mins
		return false, errors.New("block ref out of schedule")
	case nextBlockNum < forkConfig.GALACTICA && o.Type() != tx.TypeLegacy:
		// reject non legacy tx before GALACTICA
		return false, tx.ErrTxTypeNotSupported
	}

	// test features on next block
	var features tx.Features
	if nextBlockNum >= forkConfig.VIP191 {
		features.SetDelegated(true)
	}
	if err := o.TestFeatures(features); err != nil {
		return false, err
	}

	if has, err := chain.HasTransaction(o.ID(), o.BlockRef().Number()); err != nil {
		return false, err
	} else if has {
		return false, errors.New("known tx")
	}

	if dep := o.DependsOn(); dep != nil {
		txMeta, err := chain.GetTransactionMeta(*dep)
		if err != nil {
			if chain.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if txMeta.Reverted {
			return false, errors.New("dep reverted")
		}
	}

	// Tx is considered executable when the BlockRef has passed in reference to the next block.
	if o.BlockRef().Number() > nextBlockNum {
		return false, nil
	}

	checkpoint := state.NewCheckpoint()
	defer state.RevertTo(checkpoint)

	// calculate the base fee for the next block
	baseFee := galactica.CalcBaseFee(headBlock, forkConfig)
	_, _, payer, prepaid, _, err := o.resolved.BuyGas(state, nextBlockTime, baseFee)
	if err != nil {
		return false, err
	}

	if !o.executable {
		o.payer = &payer
		o.cost = prepaid
	}
	return true, nil
}

func sortTxObjsByOverallGasPriceDesc(txObjs []*txObject) {
	slices.SortFunc(txObjs, func(a, b *txObject) int {
		if cmp := b.priorityGasPrice.Cmp(a.priorityGasPrice); cmp != 0 {
			return cmp
		}
		if a.timeAdded < b.timeAdded {
			return 1
		}
		return -1
	})
}

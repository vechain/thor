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
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type TxObject struct {
	*tx.Transaction
	resolved *runtime.ResolvedTransaction

	timeAdded      int64
	localSubmitted bool          // tx is submitted locally on this node, or synced remotely from p2p.
	payer          *thor.Address // payer of the tx, either origin, delegator, or on-chain delegation payer
	cost           *big.Int      // total tx cost the payer needs to pay before execution(gas price * gas)

	// basic unit of tip price for the validator, before GALACTICA it's the overallGasPrice(provedWork included) and validator
	// gets <reward-ratio>% of the tip, after GALACTICA it's the effective priority fee per gas and validator gets 100% of the tip
	priorityGasPrice *big.Int

	executable bool // don't touch this value, will be updated by the pool
}

func ResolveTx(tx *tx.Transaction, localSubmitted bool) (*TxObject, error) {
	resolved, err := runtime.ResolveTransaction(tx)
	if err != nil {
		return nil, err
	}

	return &TxObject{
		Transaction:    tx,
		resolved:       resolved,
		timeAdded:      time.Now().UnixNano(),
		localSubmitted: localSubmitted,
	}, nil
}

func (o *TxObject) Origin() thor.Address {
	return o.resolved.Origin
}

func (o *TxObject) Delegator() *thor.Address {
	return o.resolved.Delegator
}

func (o *TxObject) Cost() *big.Int {
	return o.cost
}

func (o *TxObject) Payer() *thor.Address {
	return o.payer
}

func (o *TxObject) Executable(
	chain *chain.Chain,
	state *state.State,
	headBlock *block.Header,
	forkConfig *thor.ForkConfig,
	baseFee *big.Int,
	energyStopTime uint64,
) (bool, error) {
	// evaluate the tx on the next block as head block is already history
	nextBlockNum := headBlock.Number() + 1
	nextBlockTime := headBlock.Timestamp() + thor.BlockInterval()

	switch {
	case o.Gas() > headBlock.GasLimit():
		return false, errors.New("gas too large")
	case o.IsExpired(nextBlockNum): // Check tx expiration on top of next block
		return false, errors.New("expired")
	case o.BlockRef().Number() > nextBlockNum+uint32(5*60/thor.BlockInterval()):
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

	legacyTxBaseGasPrice, _, payer, prepaid, _, err := o.resolved.BuyGas(state, nextBlockTime, baseFee, energyStopTime)
	if err != nil {
		return false, err
	}

	// non executable -> executable, update payer, cost and priority gas price
	if !o.executable {
		o.payer = &payer
		o.cost = prepaid

		// calculate the priority gas price
		provedWork, err := o.ProvedWork(nextBlockNum, chain.GetBlockID)
		if err != nil {
			return false, err
		}
		// normalize the base fee here, set to 0 to make the func EffectivePriorityFeePerGas return overallGasPrice for before GALACTICA txs
		// before GALACTICA, the baseFeeCache.Get will return nil just like the Header.BaseFee
		if baseFee == nil {
			baseFee = big.NewInt(0)
		}
		o.priorityGasPrice = o.EffectivePriorityFeePerGas(baseFee, legacyTxBaseGasPrice, provedWork)
	}
	return true, nil
}

func sortTxObjsByPriorityGasPriceDesc(txObjs []*TxObject) {
	slices.SortFunc(txObjs, func(a, b *TxObject) int {
		if cmp := b.priorityGasPrice.Cmp(a.priorityGasPrice); cmp != 0 {
			return cmp
		}
		if a.timeAdded < b.timeAdded {
			return 1
		}
		return -1
	})
}

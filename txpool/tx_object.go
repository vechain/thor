// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"slices"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type txSource string

const (
	txSourceLocal  txSource = "local"
	txSourceRemote txSource = "remote"
	txSourceFill   txSource = "fill"
)

type txPricing struct {
	payer *thor.Address // payer of the tx, either origin, delegator, or on-chain delegation payer
	cost  *big.Int      // total tx cost the payer needs to pay before execution(gas price * gas)

	// basic unit of tip price for the validator, before GALACTICA it's the overallGasPrice(provedWork included) and validator
	// gets <reward-ratio>% of the tip, after GALACTICA it's the effective priority fee per gas and validator gets 100% of the tip
	priorityGasPrice *big.Int
}

type TxObject struct {
	*tx.Transaction
	resolved *runtime.ResolvedTransaction

	timeAdded int64
	source    txSource // where the tx came from: local, remote or fill.

	// pricing is published lock-free via an atomic snapshot, so payer/cost/priorityGasPrice
	// always move together.
	pricing atomic.Pointer[txPricing]

	executable bool // written ONLY under txObjectMap.lock; serves as the accounting gate
}

func ResolveTx(tx *tx.Transaction, localSubmitted bool) (*TxObject, error) {
	source := txSourceRemote
	if localSubmitted {
		source = txSourceLocal
	}
	return resolveTxWithSource(tx, source)
}

func resolveTxWithSource(tx *tx.Transaction, source txSource) (*TxObject, error) {
	resolved, err := runtime.ResolveTransaction(tx)
	if err != nil {
		return nil, err
	}

	return &TxObject{
		Transaction: tx,
		resolved:    resolved,
		timeAdded:   time.Now().UnixNano(),
		source:      source,
	}, nil
}

func (o *TxObject) localSubmitted() bool {
	return o.source == txSourceLocal
}

func (o *TxObject) Origin() thor.Address {
	return o.resolved.Origin
}

func (o *TxObject) Delegator() *thor.Address {
	return o.resolved.Delegator
}

func (o *TxObject) Cost() *big.Int {
	if p := o.pricing.Load(); p != nil {
		return p.cost
	}
	return nil
}

func (o *TxObject) Payer() *thor.Address {
	if p := o.pricing.Load(); p != nil {
		return p.payer
	}
	return nil
}

func (o *TxObject) priorityGasPrice() *big.Int {
	if p := o.pricing.Load(); p != nil {
		return p.priorityGasPrice
	}
	return nil
}

func (o *TxObject) setPricing(p *txPricing) { o.pricing.Store(p) }

// Evaluate performs a side-effect-free evaluation; it never mutates o. When the tx is
// executable and was not alreadyExecutable, it returns a non-nil pricing snapshot for the
// caller to publish via setPricing; otherwise pricing is nil.
func (o *TxObject) Evaluate(
	chain *chain.Chain, state *state.State, headBlock *block.Header, forkConfig *thor.ForkConfig, baseFee *big.Int, alreadyExecutable bool,
) (bool, *txPricing, error) {
	// evaluate the tx on the next block as head block is already history
	nextBlockNum := headBlock.Number() + 1
	nextBlockTime := headBlock.Timestamp() + thor.BlockInterval()

	switch {
	case o.Gas() > headBlock.GasLimit():
		return false, nil, errors.New("tx gas exceeds block gas limit")
	case o.IsExpired(nextBlockNum):
		return false, nil, errors.New("expired")
	case o.BlockRef().Number() > nextBlockNum+uint32(5*60/thor.BlockInterval()):
		// reject deferred tx which will be applied after 5mins
		return false, nil, errors.New("block ref out of schedule")
	case nextBlockNum < forkConfig.GALACTICA && o.Type() != tx.TypeLegacy:
		// reject non legacy tx before GALACTICA
		return false, nil, tx.ErrTxTypeNotSupported
	}

	// test features on next block
	var features tx.Features
	if nextBlockNum >= forkConfig.VIP191 {
		features.SetDelegated(true)
	}
	if err := o.TestFeatures(features); err != nil {
		return false, nil, err
	}

	if has, err := chain.HasTransaction(o.ID(), o.BlockRef().Number()); err != nil {
		return false, nil, err
	} else if has {
		return false, nil, errors.New("known tx")
	}

	if dep := o.DependsOn(); dep != nil {
		txMeta, err := chain.GetTransactionMeta(*dep)
		if err != nil {
			if chain.IsNotFound(err) {
				return false, nil, nil
			}
			return false, nil, err
		}
		if txMeta.Reverted {
			return false, nil, errors.New("dep reverted")
		}
	}

	// Tx is considered executable when the BlockRef has passed in reference to the next block.
	if o.BlockRef().Number() > nextBlockNum {
		return false, nil, nil
	}

	checkpoint := state.NewCheckpoint()
	defer state.RevertTo(checkpoint)

	legacyTxBaseGasPrice, _, payer, prepaid, _, err := o.resolved.BuyGas(state, nextBlockTime, baseFee)
	if err != nil {
		return false, nil, err
	}

	if alreadyExecutable {
		return true, nil, nil
	}

	// non executable -> executable: compute payer, cost and priority gas price
	provedWork, err := o.ProvedWork(nextBlockNum, chain.GetBlockID)
	if err != nil {
		return false, nil, err
	}
	// normalize the base fee here, set to 0 to make the func EffectivePriorityFeePerGas return overallGasPrice for before GALACTICA txs
	// before GALACTICA, the baseFeeCache.Get will return nil just like the Header.BaseFee
	if baseFee == nil {
		baseFee = big.NewInt(0)
	}

	return true, &txPricing{
		payer:            &payer,
		cost:             prepaid,
		priorityGasPrice: o.EffectivePriorityFeePerGas(baseFee, legacyTxBaseGasPrice, provedWork),
	}, nil
}

func sortTxObjsByPriorityGasPriceDesc(txObjs []*TxObject) {
	slices.SortFunc(txObjs, func(a, b *TxObject) int {
		if cmp := b.priorityGasPrice().Cmp(a.priorityGasPrice()); cmp != 0 {
			return cmp
		}
		if a.timeAdded < b.timeAdded {
			return 1
		}
		return -1
	})
}

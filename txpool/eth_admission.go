// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// ethAdmissionContext owns one operation's fixed chain/state snapshot and caches.
// Its prepare function produces pure ethPreparation values and does not mutate
// pool state, reservations, or events.
type ethAdmissionContext struct {
	head        *chain.BlockSummary
	state       *state.State
	stateNonces map[thor.Address]uint64
	payerFunds  map[thor.Address]*big.Int
	prepare     ethPrepare
}

func (p *EthPool) newAdmissionContext() *ethAdmissionContext {
	return p.newAdmissionContextAt(p.repo.BestBlockSummary())
}

func (p *EthPool) newAdmissionContextAt(head *chain.BlockSummary) *ethAdmissionContext {
	st := p.stater.NewState(head.Root())
	baseFee := p.baseFeeCache.Get(head.Header)
	ctx := &ethAdmissionContext{
		head:        head,
		state:       st,
		stateNonces: make(map[thor.Address]uint64),
		payerFunds:  make(map[thor.Address]*big.Int),
	}
	ctx.prepare = func(obj *TxObject) ethPreparation {
		if baseFee != nil && obj.MaxFeePerGas().Cmp(baseFee) < 0 {
			return ethPreparation{}
		}
		checkpoint := st.NewCheckpoint()
		legacyBase, _, payer, prepaid, _, err := obj.resolved.BuyGas(
			st,
			head.Header.Timestamp()+thor.BlockInterval(),
			baseFee,
		)
		st.RevertTo(checkpoint)
		if err != nil {
			return ethPreparation{err: err}
		}
		normalizedBaseFee := baseFee
		if normalizedBaseFee == nil {
			normalizedBaseFee = new(big.Int)
		}
		balance := ctx.payerFunds[payer]
		if balance == nil {
			balance, err = builtin.Energy.Native(
				st,
				head.Header.Timestamp()+thor.BlockInterval(),
			).Get(payer)
			if err != nil {
				return ethPreparation{err: err}
			}
			ctx.payerFunds[payer] = balance
		}
		return ethPreparation{
			request: reservationRequest{
				owner:   ethReservationOwner(obj.Origin(), obj.Nonce()),
				payer:   payer,
				cost:    prepaid,
				balance: balance,
			},
			viable:           true,
			priorityGasPrice: obj.EffectivePriorityFeePerGas(normalizedBaseFee, legacyBase, nil),
		}
	}
	return ctx
}

func (ctx *ethAdmissionContext) stateNonce(origin thor.Address) (uint64, error) {
	if nonce, cached := ctx.stateNonces[origin]; cached {
		return nonce, nil
	}
	nonce, err := ctx.state.GetNonce(origin)
	if err != nil {
		return 0, err
	}
	ctx.stateNonces[origin] = nonce
	return nonce, nil
}

func (p *EthPool) resolveAdmission(
	newTx *tx.Transaction,
	ctx *ethAdmissionContext,
	duplicateNoop bool,
) (*TxObject, uint64, bool, error) {
	if newTx == nil || !newTx.IsEthereumTx() {
		return nil, 0, false, badTxError{"invalid tx type for Ethereum pool"}
	}
	if p.core.GetByHash(newTx.Hash()) != nil {
		if duplicateNoop {
			return nil, 0, true, nil
		}
		return nil, 0, false, txRejectedError{errEthAlreadyKnown.Error()}
	}
	if err := validateTxBasics(p.repo, p.forkConfig, newTx); err != nil {
		return nil, 0, false, err
	}
	if p.isBlocked(newTx) {
		return nil, 0, true, nil
	}
	txObj, err := ResolveTx(newTx, false)
	if err != nil {
		return nil, 0, false, badTxError{err.Error()}
	}
	if newTx.Gas() > ctx.head.Header.GasLimit() {
		return nil, 0, false, txRejectedError{"tx gas exceeds block gas limit"}
	}
	chainView := p.repo.NewChain(ctx.head.Header.ID())
	if known, err := chainView.HasTransaction(newTx.ID(), 0); err != nil {
		return nil, 0, false, err
	} else if known {
		return nil, 0, false, txRejectedError{"known tx"}
	}
	stateNonce, err := ctx.stateNonce(txObj.Origin())
	if err != nil {
		return nil, 0, false, err
	}
	if newTx.Nonce() < stateNonce {
		return nil, 0, false, txRejectedError{errEthNonceTooLow.Error()}
	}
	return txObj, stateNonce, false, nil
}

func (p *EthPool) isBlocked(newTx *tx.Transaction) bool {
	origin, _ := newTx.Origin()
	if thor.IsOriginBlocked(origin) || (p.blocklist != nil && p.blocklist.Contains(origin)) {
		return true
	}
	delegator, _ := newTx.Delegator()
	return delegator != nil && (thor.IsOriginBlocked(*delegator) ||
		(p.blocklist != nil && p.blocklist.Contains(*delegator)))
}

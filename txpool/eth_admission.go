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

// resolveAdmissionPrecheck validates properties that need no head/state snapshot.
// skip is true for silent drops (blocklist).
func (p *EthPool) resolveAdmissionPrecheck(newTx *tx.Transaction) (*TxObject, bool, error) {
	if newTx == nil || !newTx.IsEthereumTx() {
		return nil, false, badTxError{"invalid tx type for Ethereum pool"}
	}
	if err := validateTxBasics(p.repo, p.forkConfig, newTx); err != nil {
		return nil, false, err
	}
	if p.isBlocked(newTx) {
		return nil, true, nil
	}
	txObj, err := ResolveTx(newTx, false)
	if err != nil {
		return nil, false, badTxError{err.Error()}
	}
	return txObj, false, nil
}

// resolveAdmissionWithContext validates against a fixed head/state snapshot.
func (p *EthPool) resolveAdmissionWithContext(
	txObj *TxObject,
	newTx *tx.Transaction,
	ctx *ethAdmissionContext,
) (uint64, error) {
	if newTx.Gas() > ctx.head.Header.GasLimit() {
		return 0, txRejectedError{"tx gas exceeds block gas limit"}
	}
	chainView := p.repo.NewChain(ctx.head.Header.ID())
	if known, err := chainView.HasTransaction(newTx.ID(), 0); err != nil {
		return 0, err
	} else if known {
		return 0, txRejectedError{"known tx"}
	}
	stateNonce, err := ctx.stateNonce(txObj.Origin())
	if err != nil {
		return 0, err
	}
	if newTx.Nonce() < stateNonce {
		return 0, txRejectedError{errEthNonceTooLow.Error()}
	}
	return stateNonce, nil
}

func (p *EthPool) resolveRemoteAdmission(
	newTx *tx.Transaction,
) (*TxObject, uint64, bool, *ethAdmissionContext, error) {
	if p.core.GetByHash(newTx.Hash()) != nil {
		return nil, 0, false, nil, txRejectedError{errEthAlreadyKnown.Error()}
	}
	txObj, skip, err := p.resolveAdmissionPrecheck(newTx)
	if err != nil || skip {
		return nil, 0, skip, nil, err
	}
	ctx := p.newAdmissionContext()
	stateNonce, err := p.resolveAdmissionWithContext(txObj, newTx, ctx)
	if err != nil {
		return nil, 0, false, nil, err
	}
	return txObj, stateNonce, false, ctx, nil
}

func (p *EthPool) resolveReinjectAdmission(
	newTx *tx.Transaction,
	ctx *ethAdmissionContext,
) (*TxObject, uint64, bool, error) {
	if p.core.GetByHash(newTx.Hash()) != nil {
		return nil, 0, true, nil
	}
	txObj, skip, err := p.resolveAdmissionPrecheck(newTx)
	if err != nil || skip {
		return nil, 0, skip, err
	}
	stateNonce, err := p.resolveAdmissionWithContext(txObj, newTx, ctx)
	if err != nil {
		return nil, 0, false, err
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

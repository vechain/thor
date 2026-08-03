// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"math/big"
	"sync/atomic"
)

var errEthPreparationMissing = errors.New("eth pool: prepared transaction missing")

// ethPreparationFallbacks mirrors metricEthPreparationFallback in process so the
// preparation window can be asserted to cover every commit path.
var ethPreparationFallbacks atomic.Uint64

// ethPreparation transports immutable chain/state results into a core commit; it
// does not plan sender transitions or mutate live pool state.
type ethPreparation struct {
	request          reservationRequest
	viable           bool
	err              error
	priorityGasPrice *big.Int
}

type ethPrepare func(*TxObject) ethPreparation

// ethPreparations caches the results the pre-pass gathered before the core lock
// was taken, and carries the prepare function so a miss can be served inline.
// Correctness therefore never depends on the pre-pass covering every object the
// commit ends up touching; only performance does.
type ethPreparations struct {
	byTx    map[*TxObject]ethPreparation
	prepare ethPrepare
}

func prepareEthObjects(objects []*TxObject, prepare ethPrepare) ethPreparations {
	prepared := ethPreparations{
		byTx:    make(map[*TxObject]ethPreparation, len(objects)),
		prepare: prepare,
	}
	for _, txObj := range objects {
		if _, exists := prepared.byTx[txObj]; exists {
			continue
		}
		prepared.byTx[txObj] = prepare(txObj)
	}
	return prepared
}

// get returns the cached preparation, falling back to preparing inline. The
// fallback reads chain state while the core write lock is held, so it is
// counted: a non-zero counter means the preparation window missed something.
func (p ethPreparations) get(txObj *TxObject) (ethPreparation, error) {
	if prepared, ok := p.byTx[txObj]; ok {
		return prepared, nil
	}
	if p.prepare == nil {
		return ethPreparation{}, errEthPreparationMissing
	}
	metricEthPreparationFallback().Add(1)
	ethPreparationFallbacks.Add(1)
	prepared := p.prepare(txObj)
	if p.byTx != nil {
		p.byTx[txObj] = prepared
	}
	return prepared, nil
}

func (p ethPreparation) apply(txObj *TxObject) {
	if p.err != nil || !p.viable {
		return
	}
	payer := p.request.payer
	pricing := &txPricing{
		payer: &payer,
		cost:  p.request.cost,
	}
	if p.priorityGasPrice != nil {
		pricing.priorityGasPrice = p.priorityGasPrice
	}
	txObj.setPricing(pricing)
}

// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"math/big"
)

var errEthPreparationMissing = errors.New("eth pool: prepared transaction missing")

// ethPreparation transports immutable chain/state results into a core commit; it
// does not plan sender transitions or mutate live pool state.
type ethPreparation struct {
	request          reservationRequest
	viable           bool
	err              error
	priorityGasPrice *big.Int
}

type ethPrepare func(*TxObject) ethPreparation

type ethPreparations map[*TxObject]ethPreparation

func prepareEthObjects(objects []*TxObject, prepare ethPrepare) ethPreparations {
	if len(objects) == 0 {
		return nil
	}
	prepared := make(ethPreparations, len(objects))
	for _, txObj := range objects {
		if _, exists := prepared[txObj]; exists {
			continue
		}
		prepared[txObj] = prepare(txObj)
	}
	return prepared
}

func (p ethPreparations) get(txObj *TxObject) (ethPreparation, error) {
	prepared, ok := p[txObj]
	if !ok {
		return ethPreparation{}, errEthPreparationMissing
	}
	return prepared, nil
}

func (p ethPreparation) apply(txObj *TxObject) {
	if p.err != nil || !p.viable {
		return
	}
	payer := p.request.payer
	txObj.payer = &payer
	txObj.cost = p.request.cost
	if p.priorityGasPrice != nil {
		txObj.priorityGasPrice = p.priorityGasPrice
	}
}

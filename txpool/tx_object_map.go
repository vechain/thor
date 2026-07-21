// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"math/big"
	"sync"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// txObjectMap to maintain mapping of tx hash to tx object and account quota.
// Pending VTHO cost is tracked in the shared PendingCostTracker.
type txObjectMap struct {
	lock      sync.RWMutex
	mapByHash map[thor.Bytes32]*TxObject
	mapByID   map[thor.Bytes32]*TxObject
	quota     map[thor.Address]int
	costs     *PendingCostTracker
}

func newTxObjectMap(costs *PendingCostTracker) *txObjectMap {
	return &txObjectMap{
		mapByHash: make(map[thor.Bytes32]*TxObject),
		mapByID:   make(map[thor.Bytes32]*TxObject),
		quota:     make(map[thor.Address]int),
		costs:     costs,
	}
}

func (m *txObjectMap) ContainsHash(txHash thor.Bytes32) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	_, found := m.mapByHash[txHash]
	return found
}

func (m *txObjectMap) Add(txObj *TxObject, limitPerAccount int, validatePayer func(payer thor.Address, needs *big.Int) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	hash := txObj.Hash()
	if _, found := m.mapByHash[hash]; found {
		return nil
	}

	if m.quota[txObj.Origin()] >= limitPerAccount {
		metricAccountQuotaExceeded().AddWithLabel(1, map[string]string{"type": "account"})
		return errors.New("account quota exceeded")
	}

	delegator := txObj.Delegator()
	if delegator != nil {
		if m.quota[*delegator] >= limitPerAccount {
			metricAccountQuotaExceeded().AddWithLabel(1, map[string]string{"type": "delegator"})
			return errors.New("delegator quota exceeded")
		}
	}

	if txObj.Cost() != nil {
		payer := *txObj.Payer()
		if err := m.costs.Reserve(payer, txObj.Cost(), func(needs *big.Int) error {
			return validatePayer(payer, needs)
		}); err != nil {
			return err
		}
		txObj.costReserved = true
	}

	m.quota[txObj.Origin()]++
	if delegator != nil {
		m.quota[*delegator]++
	}

	m.mapByHash[hash] = txObj
	m.mapByID[txObj.ID()] = txObj
	return nil
}

func (m *txObjectMap) GetByID(id thor.Bytes32) *TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.mapByID[id]
}

func (m *txObjectMap) GetByHash(hash thor.Bytes32) *TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.mapByHash[hash]
}

func (m *txObjectMap) RemoveByHash(txHash thor.Bytes32) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	if txObj, ok := m.mapByHash[txHash]; ok {
		if m.quota[txObj.Origin()] > 1 {
			m.quota[txObj.Origin()]--
		} else {
			delete(m.quota, txObj.Origin())
		}

		if delegator := txObj.Delegator(); delegator != nil {
			if m.quota[*delegator] > 1 {
				m.quota[*delegator]--
			} else {
				delete(m.quota, *delegator)
			}
		}

		if txObj.costReserved {
			if payer := txObj.Payer(); payer != nil {
				m.costs.Release(*payer, txObj.Cost())
			}
			txObj.costReserved = false
		}

		delete(m.mapByHash, txHash)
		delete(m.mapByID, txObj.ID())
		return true
	}
	return false
}

func (m *txObjectMap) ToTxObjects() []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	txObjs := make([]*TxObject, 0, len(m.mapByHash))
	for _, txObj := range m.mapByHash {
		txObjs = append(txObjs, txObj)
	}
	return txObjs
}

func (m *txObjectMap) ToTxs() tx.Transactions {
	m.lock.RLock()
	defer m.lock.RUnlock()

	txs := make(tx.Transactions, 0, len(m.mapByHash))
	for _, txObj := range m.mapByHash {
		txs = append(txs, txObj.Transaction)
	}
	return txs
}

func (m *txObjectMap) Fill(txObjs []*TxObject) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, txObj := range txObjs {
		if _, found := m.mapByHash[txObj.Hash()]; found {
			continue
		}
		// skip account limit check
		m.quota[txObj.Origin()]++
		if delegator := txObj.Delegator(); delegator != nil {
			m.quota[*delegator]++
		}
		m.mapByHash[txObj.Hash()] = txObj
		m.mapByID[txObj.ID()] = txObj
		// skip cost check and accumulation
	}
}

func (m *txObjectMap) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return len(m.mapByHash)
}

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

type originKey thor.Address
type delegatorKey thor.Address

// txObjectMap to maintain mapping of tx hash to tx object, account quota and pending cost.
type txObjectMap struct {
	lock      sync.RWMutex
	mapByHash map[thor.Bytes32]*txObject
	mapByID   map[thor.Bytes32]*txObject
	quota     map[interface{}]int
	cost      map[thor.Address]*big.Int
}

func newTxObjectMap() *txObjectMap {
	return &txObjectMap{
		mapByHash: make(map[thor.Bytes32]*txObject),
		mapByID:   make(map[thor.Bytes32]*txObject),
		quota:     make(map[interface{}]int),
		cost:      make(map[thor.Address]*big.Int),
	}
}

func (m *txObjectMap) ContainsHash(txHash thor.Bytes32) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	_, found := m.mapByHash[txHash]
	return found
}

func (m *txObjectMap) Add(txObj *txObject, limitPerAccount int, validatePayer func(payer thor.Address, needs *big.Int) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	hash := txObj.Hash()
	if _, found := m.mapByHash[hash]; found {
		return nil
	}

	origin := originKey(txObj.Origin())
	if m.quota[origin] >= limitPerAccount {
		return errors.New("account quota exceeded")
	}

	delegator := (*delegatorKey)(txObj.Delegator())
	if delegator != nil {
		if m.quota[*delegator] >= limitPerAccount {
			return errors.New("delegator quota exceeded")
		}
	}

	var (
		cost  *big.Int
		payer thor.Address
	)

	if txObj.Cost() != nil {
		payer = *txObj.Payer()
		pending := m.cost[payer]

		if pending == nil {
			cost = new(big.Int).Set(txObj.Cost())
		} else {
			cost = new(big.Int).Add(pending, txObj.Cost())
		}

		if err := validatePayer(payer, cost); err != nil {
			return err
		}
	}

	m.quota[origin]++
	m.mapByHash[hash] = txObj
	m.mapByID[txObj.ID()] = txObj
	if delegator != nil {
		m.quota[*delegator]++
	}
	if cost != nil {
		m.cost[payer] = cost
	}
	return nil
}

func (m *txObjectMap) GetByID(id thor.Bytes32) *txObject {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.mapByID[id]
}

func (m *txObjectMap) RemoveByHash(txHash thor.Bytes32) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	if txObj, ok := m.mapByHash[txHash]; ok {
		origin := originKey(txObj.Origin())
		if m.quota[origin] > 1 {
			m.quota[origin]--
		} else {
			delete(m.quota, origin)
		}

		if d := txObj.Delegator(); d != nil {
			delegator := delegatorKey(*d)
			if m.quota[delegator] > 1 {
				m.quota[delegator]--
			} else {
				delete(m.quota, delegator)
			}
		}

		if payer := txObj.Payer(); payer != nil {
			if pending := m.cost[*payer]; pending != nil {
				if pending.Cmp(txObj.Cost()) <= 0 {
					delete(m.cost, *payer)
				} else {
					m.cost[*payer] = new(big.Int).Sub(pending, txObj.Cost())
				}
			}
		}

		delete(m.mapByHash, txHash)
		delete(m.mapByID, txObj.ID())
		return true
	}
	return false
}

func (m *txObjectMap) UpdateCost(txObj *txObject) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if pending := m.cost[*txObj.Payer()]; pending != nil {
		m.cost[*txObj.Payer()] = new(big.Int).Add(pending, txObj.Cost())
	} else {
		m.cost[*txObj.Payer()] = new(big.Int).Set(txObj.Cost())
	}
}

func (m *txObjectMap) ToTxObjects() []*txObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	txObjs := make([]*txObject, 0, len(m.mapByHash))
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

func (m *txObjectMap) Fill(txObjs []*txObject) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, txObj := range txObjs {
		if _, found := m.mapByHash[txObj.Hash()]; found {
			continue
		}
		// skip account limit check
		m.quota[originKey(txObj.Origin())]++
		if d := txObj.Delegator(); d != nil {
			m.quota[delegatorKey(*d)]++
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

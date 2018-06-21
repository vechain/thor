// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"sync"

	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// txObjectMap to maintain mapping of ID to tx object, and account quota.
type txObjectMap struct {
	lock     sync.RWMutex
	txObjMap map[thor.Bytes32]*txObject
	quota    map[thor.Address]int
}

func newTxObjectMap() *txObjectMap {
	return &txObjectMap{
		txObjMap: make(map[thor.Bytes32]*txObject),
		quota:    make(map[thor.Address]int),
	}
}

func (m *txObjectMap) Contains(txID thor.Bytes32) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	_, found := m.txObjMap[txID]
	return found
}

func (m *txObjectMap) Add(txObj *txObject, limitPerAccount int) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if _, found := m.txObjMap[txObj.ID()]; found {
		return nil
	}

	if m.quota[txObj.Origin()] >= limitPerAccount {
		return errors.New("account quota exceeded")
	}

	m.quota[txObj.Origin()]++
	m.txObjMap[txObj.ID()] = txObj
	return nil
}

func (m *txObjectMap) Remove(txID thor.Bytes32) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	if txObj, ok := m.txObjMap[txID]; ok {
		if m.quota[txObj.Origin()] > 1 {
			m.quota[txObj.Origin()]--
		} else {
			delete(m.quota, txObj.Origin())
		}
		delete(m.txObjMap, txID)
		return true
	}
	return false
}

func (m *txObjectMap) ToTxObjects() []*txObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	txObjs := make([]*txObject, 0, len(m.txObjMap))
	for _, txObj := range m.txObjMap {
		txObjs = append(txObjs, txObj)
	}
	return txObjs
}

func (m *txObjectMap) ToTxs() tx.Transactions {
	m.lock.RLock()
	defer m.lock.RUnlock()

	txs := make(tx.Transactions, 0, len(m.txObjMap))
	for _, txObj := range m.txObjMap {
		txs = append(txs, txObj.Transaction)
	}
	return txs
}

func (m *txObjectMap) Fill(txObjs []*txObject) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, txObj := range txObjs {
		if _, found := m.txObjMap[txObj.ID()]; found {
			continue
		}
		// skip account limit check

		m.quota[txObj.Origin()]++
		m.txObjMap[txObj.ID()] = txObj
	}
}

func (m *txObjectMap) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return len(m.txObjMap)
}

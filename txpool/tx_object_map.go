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
// Executable transaction cost is reserved in the shared costTracker.
type txObjectMap struct {
	lock            sync.RWMutex
	mapByHash       map[thor.Bytes32]*TxObject
	mapByID         map[thor.Bytes32]*TxObject
	quota           map[thor.Address]int
	executableCount int
	costs           *costTracker
}

func newTxObjectMap(costs *costTracker) *txObjectMap {
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

func (m *txObjectMap) Add(txObj *TxObject, limitPerAccount int, balance *big.Int) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	hash := txObj.Hash()
	if _, found := m.mapByHash[hash]; found {
		return nil
	}
	if err := m.checkQuotaLocked(txObj, limitPerAccount); err != nil {
		return err
	}

	if txObj.Cost() != nil {
		payer := *txObj.Payer()
		if err := m.costs.reserve(vechainReservationOwner(hash), payer, txObj.Cost(), balance); err != nil {
			return err
		}
	}

	m.insertLocked(hash, txObj)
	return nil
}

func (m *txObjectMap) checkQuotaLocked(txObj *TxObject, limitPerAccount int) error {
	if m.quota[txObj.Origin()] >= limitPerAccount {
		metricAccountQuotaExceeded().AddWithLabel(1, map[string]string{"type": "account"})
		return errors.New("account quota exceeded")
	}
	if delegator := txObj.Delegator(); delegator != nil {
		if m.quota[*delegator] >= limitPerAccount {
			metricAccountQuotaExceeded().AddWithLabel(1, map[string]string{"type": "delegator"})
			return errors.New("delegator quota exceeded")
		}
	}
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

// ReserveCost reserves txObj's executable cost and marks it executable if it
// is still in the map.
// Holding the map lock closes the race between wash promotion and removal;
// costTracker is a leaf lock, so this is the required lock order.
func (m *txObjectMap) ReserveCost(txObj *TxObject, balance *big.Int) (bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if current := m.mapByHash[txObj.Hash()]; current != txObj {
		return false, nil
	}
	err := m.costs.reserve(
		vechainReservationOwner(txObj.Hash()),
		*txObj.Payer(),
		txObj.Cost(),
		balance,
	)
	if err == nil {
		m.setExecutableLocked(txObj, true)
	}
	return err == nil, err
}

func (m *txObjectMap) RemoveByHash(txHash thor.Bytes32) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	txObj, ok := m.mapByHash[txHash]
	if !ok {
		return false
	}
	m.removeLocked(txHash, txObj)
	return true
}

// RemoveByHashAndID removes a transaction only when the object at txHash has
// txID. The check and removal are atomic with respect to map mutations.
func (m *txObjectMap) RemoveByHashAndID(txHash, txID thor.Bytes32) *TxObject {
	m.lock.Lock()
	defer m.lock.Unlock()

	txObj := m.mapByHash[txHash]
	if txObj == nil || txObj.ID() != txID {
		return nil
	}
	m.removeLocked(txHash, txObj)
	return txObj
}

func (m *txObjectMap) insertLocked(hash thor.Bytes32, txObj *TxObject) {
	m.quota[txObj.Origin()]++
	if delegator := txObj.Delegator(); delegator != nil {
		m.quota[*delegator]++
	}
	m.mapByHash[hash] = txObj
	m.mapByID[txObj.ID()] = txObj
	if txObj.executable {
		m.executableCount++
	}
}

func (m *txObjectMap) setExecutableLocked(txObj *TxObject, executable bool) {
	if txObj.executable == executable {
		return
	}
	txObj.executable = executable
	if executable {
		m.executableCount++
	} else {
		m.executableCount--
	}
}

func (m *txObjectMap) removeLocked(txHash thor.Bytes32, txObj *TxObject) {
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

	delete(m.mapByHash, txHash)
	if m.mapByID[txObj.ID()] == txObj {
		delete(m.mapByID, txObj.ID())
	}
	if txObj.executable {
		m.executableCount--
	}
	if err := m.costs.release(vechainReservationOwner(txHash)); err != nil {
		logger.Error("failed to release transaction cost", "hash", txHash, "err", err)
	}
	return true
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

// executableSnapshot preserves the order of txs while copying immutable merge
// keys under one map lock. Transactions removed since the wash are omitted.
func (m *txObjectMap) executableSnapshot(txs tx.Transactions) *vechainExecutablesSnapshot {
	m.lock.RLock()
	defer m.lock.RUnlock()

	snapshot := make(vechainExecutablesSnapshot, 0, len(txs))
	for _, trx := range txs {
		txObj := m.mapByID[trx.ID()]
		if txObj == nil || txObj.priorityGasPrice == nil {
			continue
		}
		snapshot = append(snapshot, executableTxFromObject(txObj))
	}
	return &snapshot
}

func (m *txObjectMap) Fill(txObjs []*TxObject) {
	m.lock.Lock()
	defer m.lock.Unlock()

	inserted := make([]*TxObject, 0, len(txObjs))
	for _, txObj := range txObjs {
		hash := txObj.Hash()
		if _, found := m.mapByHash[hash]; found {
			continue
		}
		// skip account limit check
		m.insertLocked(hash, txObj)
		// skip cost check and accumulation
	}
}

// Counts returns a consistent view of total and executable transactions.
func (m *txObjectMap) Counts() (total, executable int) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.mapByHash), m.executableCount
}

func (m *txObjectMap) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return len(m.mapByHash)
}

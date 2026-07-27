// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"slices"

	"github.com/vechain/thor/v2/thor"
)

// Planning reads and simulates sender transitions to select preparation work;
// it never mutates live core state, transaction fields, or cost reservations.
// ethSenderView is a read-only planning copy of an ethSender. Its maps may be
// mutated freely because TxObject fields are never changed during planning.
type ethSenderView struct {
	stateNonce uint64
	pending    map[uint64]*TxObject
	queue      map[uint64]*TxObject
}

func newEthSenderView(sender *ethSender, stateNonce uint64) *ethSenderView {
	view := &ethSenderView{
		stateNonce: stateNonce,
		pending:    make(map[uint64]*TxObject),
		queue:      make(map[uint64]*TxObject),
	}
	if sender == nil {
		return view
	}
	view.stateNonce = sender.stateNonce
	for nonce, txObj := range sender.pending {
		view.pending[nonce] = txObj
	}
	for nonce, txObj := range sender.queue {
		view.queue[nonce] = txObj
	}
	return view
}

func (v *ethSenderView) poolNonce() uint64 {
	return v.stateNonce + uint64(len(v.pending))
}

func (v *ethSenderView) get(nonce uint64) *TxObject {
	if txObj := v.pending[nonce]; txObj != nil {
		return txObj
	}
	return v.queue[nonce]
}

func (v *ethSenderView) syncStateNonce(stateNonce uint64) {
	if stateNonce < v.stateNonce {
		for nonce, txObj := range v.pending {
			v.queue[nonce] = txObj
			delete(v.pending, nonce)
		}
		v.stateNonce = stateNonce
		return
	}
	if stateNonce == v.stateNonce {
		return
	}
	for nonce := range v.pending {
		if nonce < stateNonce {
			delete(v.pending, nonce)
		}
	}
	for nonce := range v.queue {
		if nonce < stateNonce {
			delete(v.queue, nonce)
		}
	}
	v.stateNonce = stateNonce
}

func (v *ethSenderView) resetStateNonce(stateNonce uint64) {
	for nonce, txObj := range v.pending {
		delete(v.pending, nonce)
		if nonce >= stateNonce {
			v.queue[nonce] = txObj
		}
	}
	for nonce := range v.queue {
		if nonce < stateNonce {
			delete(v.queue, nonce)
		}
	}
	v.stateNonce = stateNonce
}

func (v *ethSenderView) demoteFrom(nonce uint64) {
	for pendingNonce, txObj := range v.pending {
		if pendingNonce >= nonce {
			v.queue[pendingNonce] = txObj
			delete(v.pending, pendingNonce)
		}
	}
}

func (v *ethSenderView) dropPending(nonce uint64) {
	if v.pending[nonce] == nil {
		return
	}
	delete(v.pending, nonce)
	if nonce < ^uint64(0) {
		v.demoteFrom(nonce + 1)
	}
}

func (v *ethSenderView) removeExpired(options ethWashOptions) {
	if options.maxLifetime <= 0 {
		return
	}
	expired := func(txObj *TxObject) bool {
		return !txObj.localSubmitted &&
			options.now > txObj.timeAdded &&
			options.now-txObj.timeAdded > int64(options.maxLifetime)
	}
	for nonce := v.stateNonce; nonce < v.poolNonce(); nonce++ {
		if txObj := v.pending[nonce]; txObj != nil && expired(txObj) {
			v.dropPending(nonce)
			break
		}
	}
	for nonce, txObj := range v.queue {
		if expired(txObj) {
			delete(v.queue, nonce)
		}
	}
}

func (v *ethSenderView) pendingObjects() []*TxObject {
	objects := make([]*TxObject, 0, len(v.pending))
	for nonce := v.stateNonce; nonce < v.poolNonce(); nonce++ {
		if txObj := v.pending[nonce]; txObj != nil {
			objects = append(objects, txObj)
		}
	}
	return objects
}

func (v *ethSenderView) promotionObjects(pendingLimit int) []*TxObject {
	if pendingLimit < 0 {
		pendingLimit = len(v.pending) + len(v.queue)
	}
	objects := make([]*TxObject, 0, max(0, pendingLimit-len(v.pending)))
	next := v.poolNonce()
	for len(v.pending)+len(objects) < pendingLimit {
		txObj := v.queue[next]
		if txObj == nil {
			break
		}
		objects = append(objects, txObj)
		next++
	}
	return objects
}

func sortedViewQueue(view *ethSenderView) []*TxObject {
	nonces := make([]uint64, 0, len(view.queue))
	for nonce := range view.queue {
		nonces = append(nonces, nonce)
	}
	slices.Sort(nonces)
	objects := make([]*TxObject, 0, len(nonces))
	for _, nonce := range nonces {
		objects = append(objects, view.queue[nonce])
	}
	return objects
}

func (m *ethPoolCore) addPreparationObjects(
	txObj *TxObject,
	stateNonce uint64,
	pendingLimit int,
	priceBump uint64,
) []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if m.allByHash[txObj.Hash()] != nil {
		return nil
	}
	origin := txObj.Origin()
	sender := m.senders[origin]
	if sender == nil {
		if txObj.Nonce() == stateNonce && pendingLimit > 0 {
			return []*TxObject{txObj}
		}
		return nil
	}
	if stateNonce == sender.stateNonce {
		incumbent := sender.get(txObj.Nonce())
		if incumbent != nil && !isFeeBumpSufficient(incumbent, txObj, priceBump) {
			return nil
		}
		replacePending := incumbent != nil && sender.pending[txObj.Nonce()] != nil
		canEnterPending := replacePending ||
			(txObj.Nonce() == sender.poolNonce() && len(sender.pending) < pendingLimit)
		if !canEnterPending {
			return senderPromotionObjects(sender, pendingLimit, sender.poolNonce(), len(sender.pending))
		}

		objects := make([]*TxObject, 0, 1+max(0, pendingLimit-len(sender.pending)))
		objects = append(objects, txObj)
		if !replacePending {
			objects = append(objects, senderPromotionObjects(
				sender,
				pendingLimit,
				sender.poolNonce()+1,
				len(sender.pending)+1,
			)...)
		} else {
			objects = append(objects, senderPromotionObjects(
				sender,
				pendingLimit,
				sender.poolNonce(),
				len(sender.pending),
			)...)
		}
		return objects
	}

	view := newEthSenderView(sender, stateNonce)
	view.syncStateNonce(stateNonce)
	if txObj.Nonce() < view.stateNonce {
		return nil
	}
	incumbent := view.get(txObj.Nonce())
	if incumbent != nil && !isFeeBumpSufficient(incumbent, txObj, priceBump) {
		return nil
	}

	replacePending := incumbent != nil && view.pending[txObj.Nonce()] != nil
	canEnterPending := replacePending ||
		(txObj.Nonce() == view.poolNonce() && len(view.pending) < pendingLimit)
	objects := make([]*TxObject, 0, 1+max(0, pendingLimit-len(view.pending)))
	if canEnterPending {
		objects = append(objects, txObj)
		view.pending[txObj.Nonce()] = txObj
		delete(view.queue, txObj.Nonce())
	} else {
		if replacePending {
			view.demoteFrom(txObj.Nonce())
		}
		view.queue[txObj.Nonce()] = txObj
		delete(view.pending, txObj.Nonce())
	}
	return append(objects, view.promotionObjects(pendingLimit)...)
}

func senderPromotionObjects(
	sender *ethSender,
	pendingLimit int,
	next uint64,
	pendingCount int,
) []*TxObject {
	if pendingLimit < 0 {
		pendingLimit = len(sender.pending) + len(sender.queue)
	}
	var objects []*TxObject
	for pendingCount+len(objects) < pendingLimit {
		txObj := sender.queue[next]
		if txObj == nil {
			break
		}
		objects = append(objects, txObj)
		next++
	}
	return objects
}

func (m *ethPoolCore) syncPreparationObjects(
	stateNonces map[thor.Address]uint64,
	pendingLimit int,
) []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var objects []*TxObject
	for _, origin := range sortedEthOrigins(stateNonces) {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		if stateNonces[origin] == sender.stateNonce {
			objects = append(objects, senderPromotionObjects(
				sender,
				pendingLimit,
				sender.poolNonce(),
				len(sender.pending),
			)...)
			continue
		}
		view := newEthSenderView(sender, stateNonces[origin])
		view.syncStateNonce(stateNonces[origin])
		objects = append(objects, view.promotionObjects(pendingLimit)...)
	}
	return objects
}

func (m *ethPoolCore) washPreparationObjects(
	stateNonces map[thor.Address]uint64,
	options ethWashOptions,
) []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var objects []*TxObject
	for _, origin := range m.sortedOriginsLocked() {
		stateNonce, present := stateNonces[origin]
		if !present {
			continue
		}
		sender := m.senders[origin]
		if stateNonce == sender.stateNonce && !senderHasExpired(sender, options) {
			for nonce := sender.stateNonce; nonce < sender.poolNonce(); nonce++ {
				if txObj := sender.pending[nonce]; txObj != nil {
					objects = append(objects, txObj)
				}
			}
			objects = append(objects, senderPromotionObjects(
				sender,
				options.pendingLimit,
				sender.poolNonce(),
				len(sender.pending),
			)...)
			continue
		}
		view := newEthSenderView(sender, stateNonce)
		view.syncStateNonce(stateNonce)
		view.removeExpired(options)
		objects = append(objects, view.pendingObjects()...)
		objects = append(objects, view.promotionObjects(options.pendingLimit)...)
	}
	return objects
}

func senderHasExpired(sender *ethSender, options ethWashOptions) bool {
	if options.maxLifetime <= 0 {
		return false
	}
	expired := func(txObj *TxObject) bool {
		return !txObj.localSubmitted &&
			options.now > txObj.timeAdded &&
			options.now-txObj.timeAdded > int64(options.maxLifetime)
	}
	for _, txObj := range sender.pending {
		if expired(txObj) {
			return true
		}
	}
	for _, txObj := range sender.queue {
		if expired(txObj) {
			return true
		}
	}
	return false
}

func (m *ethPoolCore) forkPreparationObjects(
	candidates []ethForkCandidate,
	stateNonces map[thor.Address]uint64,
) []*TxObject {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var objects []*TxObject
	seenOrigins := make(map[thor.Address]struct{}, len(stateNonces)+len(candidates))
	for _, origin := range sortedEthOrigins(stateNonces) {
		stateNonce := stateNonces[origin]
		seenOrigins[origin] = struct{}{}
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		view := newEthSenderView(sender, stateNonce)
		view.resetStateNonce(stateNonce)
		objects = append(objects, sortedViewQueue(view)...)
	}
	for _, candidate := range candidates {
		objects = append(objects, candidate.txObj)
		origin := candidate.txObj.Origin()
		if _, seen := seenOrigins[origin]; seen {
			continue
		}
		seenOrigins[origin] = struct{}{}
		if sender := m.senders[origin]; sender != nil {
			view := newEthSenderView(sender, candidate.stateNonce)
			view.resetStateNonce(candidate.stateNonce)
			objects = append(objects, sortedViewQueue(view)...)
		}
	}
	return objects
}

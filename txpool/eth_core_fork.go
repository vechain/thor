// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"

	"github.com/vechain/thor/v2/thor"
)

// Fork reconciliation resets and rebuilds affected sender state atomically;
// it does not discover forks, load state, or publish pool events.
type ethForkCandidate struct {
	txObj      *TxObject
	stateNonce uint64
}

type ethForkResult struct {
	txObj      *TxObject
	executable bool
	promoted   []*TxObject
	err        error
}

func (m *ethPoolCore) reconcileFork(
	candidates []ethForkCandidate,
	stateNonces map[thor.Address]uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
) ([]ethForkResult, error) {
	results, _, err := m.reconcileForkWithTransitions(
		candidates, stateNonces, globalLimit, pendingLimit, queueLimit, priceBump, prepare,
	)
	return results, err
}

func (m *ethPoolCore) reconcileForkWithTransitions(
	candidates []ethForkCandidate,
	stateNonces map[thor.Address]uint64,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepare ethPrepare,
) ([]ethForkResult, []*TxObject, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()

	objects := m.forkPreparationWindow(candidates, stateNonces, pendingLimit)
	prepared := prepareEthObjects(objects, prepare)

	m.lock.Lock()
	defer m.lock.Unlock()

	origins := sortedEthOrigins(stateNonces)
	wasExecutable := m.executableObjectsLocked(forkScopeOrigins(origins, candidates))
	if err := m.resetForkSendersLocked(origins, stateNonces); err != nil {
		return nil, nil, err
	}

	results, err := m.promoteForkSendersLocked(origins, wasExecutable, pendingLimit, prepared)
	if err != nil {
		return nil, nil, err
	}
	candidateResults, err := m.addForkCandidatesLocked(
		candidates,
		wasExecutable,
		globalLimit,
		pendingLimit,
		queueLimit,
		priceBump,
		prepared,
	)
	if err != nil {
		return nil, nil, err
	}
	results = append(results, candidateResults...)
	m.pruneForkSendersLocked(forkScopeOrigins(origins, candidates))
	return results, m.retainedDemotionsLocked(wasExecutable), nil
}

// forkScopeOrigins lists every sender a fork reconcile can mutate: the origins
// carried by the state snapshot, plus any candidate origin missing from it.
// Candidate origins normally appear in stateNonces already, but they are
// included defensively rather than by scanning every sender in the pool.
func forkScopeOrigins(origins []thor.Address, candidates []ethForkCandidate) []thor.Address {
	scope := make([]thor.Address, 0, len(origins)+len(candidates))
	scope = append(scope, origins...)
	seen := make(map[thor.Address]struct{}, len(origins)+len(candidates))
	for _, origin := range origins {
		seen[origin] = struct{}{}
	}
	for _, candidate := range candidates {
		origin := candidate.txObj.Origin()
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		scope = append(scope, origin)
	}
	return scope
}

// resetForkSendersLocked releases every stale reservation before promotion.
func (m *ethPoolCore) resetForkSendersLocked(
	origins []thor.Address,
	stateNonces map[thor.Address]uint64,
) error {
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		settled, releases := sender.resetStateNonce(stateNonces[origin])
		if err := m.costs.release(releases...); err != nil {
			return err
		}
		for _, old := range settled {
			delete(m.allByHash, old.Hash())
		}
	}
	return nil
}

func (m *ethPoolCore) promoteForkSendersLocked(
	origins []thor.Address,
	wasExecutable map[thor.Bytes32]*TxObject,
	pendingLimit int,
	prepared ethPreparations,
) ([]ethForkResult, error) {
	var results []ethForkResult
	for _, origin := range origins {
		sender := m.senders[origin]
		if sender == nil {
			continue
		}
		promoted, err := m.promoteLocked(sender, pendingLimit, prepared)
		if err != nil {
			return nil, err
		}
		for _, txObj := range m.newPromotionsLocked(promoted, wasExecutable) {
			results = append(results, ethForkResult{txObj: txObj, executable: true})
		}
	}
	return results, nil
}

func (m *ethPoolCore) addForkCandidatesLocked(
	candidates []ethForkCandidate,
	wasExecutable map[thor.Bytes32]*TxObject,
	globalLimit int,
	pendingLimit int,
	queueLimit int,
	priceBump uint64,
	prepared ethPreparations,
) ([]ethForkResult, error) {
	results := make([]ethForkResult, 0, len(candidates))
	for _, candidate := range candidates {
		executable, promoted, err := m.addLocked(
			candidate.txObj,
			candidate.stateNonce,
			globalLimit,
			pendingLimit,
			queueLimit,
			priceBump,
			prepared,
		)
		if errors.Is(err, errCostTrackerState) || errors.Is(err, errInvalidCost) {
			return nil, err
		}
		results = append(results, ethForkResult{
			txObj:      candidate.txObj,
			executable: executable,
			promoted:   m.newPromotionsLocked(promoted, wasExecutable),
			err:        err,
		})
	}
	return results, nil
}

func (m *ethPoolCore) pruneForkSendersLocked(origins []thor.Address) {
	for _, origin := range origins {
		if sender := m.senders[origin]; sender != nil && sender.isEmpty() {
			delete(m.senders, origin)
		}
	}
}

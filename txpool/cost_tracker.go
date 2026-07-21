// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"math/big"
	"sync"

	"github.com/vechain/thor/v2/thor"
)

var (
	errInsufficientEnergy = errors.New("insufficient energy for overall pending cost")
	errInvalidCost        = errors.New("cost tracker: cost must be non-negative")
	errCostTrackerState   = errors.New("cost tracker: inconsistent reservation state")
)

// costTracker accounts for the VTHO reserved by executable transactions across
// all sub-pools. It is a leaf lock: its methods only mutate in-memory state and
// never call back into a pool or the chain state.
type costTracker struct {
	mu           sync.Mutex
	pending      map[thor.Address]*big.Int
	reservations map[reservationOwner]reservation
}

type reservationFamily uint8

type reservationOwner struct {
	family reservationFamily
	origin thor.Address
	nonce  uint64
	hash   thor.Bytes32
}

type reservation struct {
	payer thor.Address
	cost  *big.Int
}

type reservationRequest struct {
	owner   reservationOwner
	payer   thor.Address
	cost    *big.Int
	balance *big.Int
}

const (
	reservationVeChain reservationFamily = iota + 1
	reservationEth
)

type reconcileMode uint8

const (
	acceptAffordablePrefix reconcileMode = iota
	requireAllReservations
)

func vechainReservationOwner(hash thor.Bytes32) reservationOwner {
	return reservationOwner{family: reservationVeChain, hash: hash}
}

func ethReservationOwner(origin thor.Address, nonce uint64) reservationOwner {
	return reservationOwner{family: reservationEth, origin: origin, nonce: nonce}
}

func newCostTracker() *costTracker {
	return &costTracker{
		pending:      make(map[thor.Address]*big.Int),
		reservations: make(map[reservationOwner]reservation),
	}
}

// reserve atomically replaces owner's existing reservation. The caller must
// provide a balance read from the state snapshot used to validate the tx.
func (t *costTracker) reserve(owner reservationOwner, payer thor.Address, cost, balance *big.Int) error {
	_, err := t.reconcile(nil, []reservationRequest{{
		owner:   owner,
		payer:   payer,
		cost:    cost,
		balance: balance,
	}}, requireAllReservations)
	return err
}

// release removes reservations by owner. Unknown owners are ignored, making
// transaction removal idempotent.
func (t *costTracker) release(owners ...reservationOwner) error {
	_, err := t.reconcile(owners, nil, requireAllReservations)
	return err
}

// reconcile atomically removes releaseOwners and replaces every owner present
// in desired. It returns the affordable desired prefix. In require-all mode,
// accepting only a prefix rolls the operation back.
func (t *costTracker) reconcile(releaseOwners []reservationOwner, desired []reservationRequest, mode reconcileMode) (int, error) {
	if err := validateReservations(desired); err != nil {
		return 0, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	ownersToReplace := make(map[reservationOwner]struct{}, len(releaseOwners)+len(desired))
	for _, owner := range releaseOwners {
		ownersToReplace[owner] = struct{}{}
	}
	for _, charge := range desired {
		ownersToReplace[charge.owner] = struct{}{}
	}

	old := make(map[reservationOwner]reservation, len(ownersToReplace))
	releasedByPayer := make(map[thor.Address]*big.Int)
	for owner := range ownersToReplace {
		if charge, ok := t.reservations[owner]; ok {
			old[owner] = charge
			addCost(releasedByPayer, charge.payer, charge.cost)
		}
	}
	for payer, cost := range releasedByPayer {
		if pending := t.pending[payer]; pending == nil || pending.Cmp(cost) < 0 {
			return 0, errCostTrackerState
		}
	}
	for owner, charge := range old {
		t.releaseCost(charge.payer, charge.cost)
		delete(t.reservations, owner)
	}

	accepted := 0
	for _, charge := range desired {
		if err := t.reserveCost(charge.payer, charge.cost, charge.balance); err != nil {
			if mode == acceptAffordablePrefix {
				return accepted, nil
			}
			t.rollback(old, desired[:accepted])
			return 0, err
		}
		t.reservations[charge.owner] = reservation{
			payer: charge.payer,
			cost:  new(big.Int).Set(charge.cost),
		}
		accepted++
	}
	return accepted, nil
}

func validateReservations(reservations []reservationRequest) error {
	owners := make(map[reservationOwner]struct{}, len(reservations))
	for _, charge := range reservations {
		if charge.cost == nil || charge.cost.Sign() < 0 {
			return errInvalidCost
		}
		if charge.balance == nil {
			return errInsufficientEnergy
		}
		if _, duplicate := owners[charge.owner]; duplicate {
			return errors.New("cost tracker: duplicate reservation owner")
		}
		owners[charge.owner] = struct{}{}
	}
	return nil
}

func (t *costTracker) reserveCost(payer thor.Address, cost, balance *big.Int) error {
	needs := new(big.Int).Set(cost)
	if pending := t.pending[payer]; pending != nil {
		needs.Add(needs, pending)
	}
	if balance.Cmp(needs) < 0 {
		return errInsufficientEnergy
	}
	if needs.Sign() == 0 {
		delete(t.pending, payer)
		return nil
	}
	t.pending[payer] = needs
	return nil
}

func (t *costTracker) releaseCost(payer thor.Address, cost *big.Int) {
	if cost.Sign() == 0 {
		return
	}
	pending := t.pending[payer]
	if pending.Cmp(cost) == 0 {
		delete(t.pending, payer)
		return
	}
	t.pending[payer] = new(big.Int).Sub(pending, cost)
}

func (t *costTracker) rollback(old map[reservationOwner]reservation, accepted []reservationRequest) {
	for _, charge := range accepted {
		t.releaseCost(charge.payer, charge.cost)
		delete(t.reservations, charge.owner)
	}
	for owner, charge := range old {
		addCost(t.pending, charge.payer, charge.cost)
		t.reservations[owner] = charge
	}
}

func addCost(costs map[thor.Address]*big.Int, payer thor.Address, cost *big.Int) {
	if cost.Sign() == 0 {
		return
	}
	if current := costs[payer]; current != nil {
		costs[payer] = new(big.Int).Add(current, cost)
		return
	}
	costs[payer] = new(big.Int).Set(cost)
}

func (t *costTracker) pendingCost(payer thor.Address) *big.Int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cost := t.pending[payer]; cost != nil {
		return new(big.Int).Set(cost)
	}
	return new(big.Int)
}

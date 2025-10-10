// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"fmt"
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

type Status struct {
	Active  bool // indicates if the staker contract is currently active
	Updates bool // indicates if there are updates to the staker contract
}

// SyncPOS checks the status of the staker contract and updates its state based on the current block number.
// It returns a Status object containing the activation status and the current leader group.
// If the staker contract is not active, it attempts to transition to dPoS on transition blocks.
// If the staker contract is active, it performs housekeeping on epoch blocks.
func (s *Staker) SyncPOS(forkConfig *thor.ForkConfig, current uint32) (Status, error) {
	status := Status{}
	// still on PoA
	if forkConfig.HAYABUSA+thor.HayabusaTP() > current {
		return status, nil
	}

	var err error
	var activated bool

	// check if the staker contract is active
	status.Active, err = s.IsPoSActive()
	if err != nil {
		return status, err
	}

	// attempt to transition if we're on a transition block and the staker contract is not active
	if !status.Active && (thor.HayabusaTP() == 0 || (current-forkConfig.HAYABUSA)%thor.HayabusaTP() == 0) {
		activated, err = s.transition(current)
		if err != nil {
			return status, fmt.Errorf("failed to transition to dPoS: %w", err)
		}
		if activated {
			status.Active = true
			status.Updates = true
		}
	}

	// perform housekeeping if the staker contract is active
	if status.Active && !activated {
		status.Updates, err = s.Housekeep(current)
		if err != nil {
			return status, err
		}
	}

	return status, nil
}

func ToVET(wei *big.Int) uint64 {
	return new(big.Int).Div(wei, bigE18).Uint64()
}

func ToWei(vet uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(vet), bigE18)
}

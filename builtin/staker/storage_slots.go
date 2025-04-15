// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import "github.com/vechain/thor/v2/thor"

var (
	slotLockedVET         = nameToSlot("total-stake")
	slotValidations       = nameToSlot("validations")
	slotValidationLookups = nameToSlot("validator-lookups")
	slotDelegations       = nameToSlot("delegations")
	slotDelegators        = nameToSlot("delegators")
	// active validators linked list
	slotActiveTail      = nameToSlot("validators-active-tail")
	slotActiveHead      = nameToSlot("validators-active-head")
	slotActiveGroupSize = nameToSlot("validators-active-group-size")
	// queued validators linked list
	slotQueuedHead      = nameToSlot("validators-queued-head")
	slotQueuedTail      = nameToSlot("validators-queued-tail")
	slotQueuedGroupSize = nameToSlot("validators-queued-group-size")
	// init params
	slotLowStakingPeriod    = nameToSlot("staker-low-staking-period")
	slotMediumStakingPeriod = nameToSlot("staker-medium-staking-period")
	slotHighStakingPeriod   = nameToSlot("staker-high-staking-period")
)

func nameToSlot(name string) thor.Bytes32 {
	return thor.BytesToBytes32([]byte(name))
}

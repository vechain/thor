// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

type Status = uint8

const (
	StatusUnknown  = Status(iota) // 0 -> default value
	StatusQueued                  // Once on the queue
	StatusActive                  // When activated by protocol
	StatusCooldown                // When in cooldown
	StatusExit                    // Validator should not be used again
)

type (
	Validator struct {
		Endorsor thor.Address // the address providing the stake
		Expiry   uint32
		Stake    *big.Int      // the stake of the validator
		Weight   *big.Int      // stake + total stake from delegators
		Next     *thor.Address `rlp:"nil"` // doubly linked list
		Prev     *thor.Address `rlp:"nil"` // doubly linked list
		Status   Status        // status of the validator
		Online   bool          // whether the validator is online or not
	}

	previousExit struct {
		PreviousExit uint32
	}
)

// IsEmpty returns whether the entry can be treated as empty.
func (v *Validator) IsEmpty() bool {
	emptyStake := v.Stake == nil || v.Stake.Sign() == 0
	emptyWeight := v.Weight == nil || v.Weight.Sign() == 0

	return emptyStake && emptyWeight && v.Status == StatusUnknown && v.Prev == nil && v.Next == nil
}

// IsLinked returns whether the entry is linked.
func (v *Validator) IsLinked() bool {
	return v.Prev != nil || v.Next != nil
}

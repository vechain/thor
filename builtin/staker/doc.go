// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

// Package staker implements the builtin staker contract.
//
// It contains the core logic for the staker contract, including the staker's
// state, the staker's methods, and the staker's events.
//
// The methods are designed for two purposes:
// 1. To be called by the user, which will be bridged by the staker.sol contract
// 2. To be called by system for resolve the staking state which is defined in protocol.go
//      Actions are defined as the following:
//		- Activate Proof of Stake after transition period
//		- Handle renewal that trigger by user(validation and delegation)
// 			- Validation Increase/Decrease stake
// 			- Delegation Add/Signal Exit when validation is active
//		- Handles exit after validation signaled exit
//		- Handles eviction after validation is offline for a defined threshold

// The staker contract built a self managed storage layout, which are
// - Single slot storage:
// 	- Locked stake: 'total-weighted-stake' -> [stake(uint64), weight(uint64)]
// 	- Queued stake: 'queued-stake' -> stake(uint64)
// 	- Delegation ID counter: 'delegations-counter' -> uint256
// - Mapping storage:
// 	- Validations: Blake2b(validator(address) + 'validations' -> Validation
// 	- Delegations: Blake2b(delegationID(uint256) + 'delegations') -> Delegation
// 	- Aggregations: Blake2b(validator(address) + 'aggregations') -> Aggregation
// 	- Rewards by period: Blake2b([period(4 bytes) + validator(20 bytes) + 8 bytes zero] + 'period-rewards') -> reward(uint256)
// 	- Exit block to validator: Blake2b([ block number(4 bytes) + 28 bytes zero ] + 'exit-epochs') -> validator(address)
// - List stats storage:
// 	- Active list head(single slot): 'validations-active-head' -> address
// 	- Active list tail(single slot): 'validations-active-tail' -> address
// 	- Active list group size(single slot): 'validations-active-group-size' -> uint64
// 	- Queued list head(single slot): 'validations-queued-head' -> address
// 	- Queued list tail(single slot): 'validations-queued-tail' -> address
// 	- Queued list group size(single slot): 'validations-queued-group-size' -> uint64
// - Renewal list:
// 	- Renewal list head(single slot): 'validations-renewal-head' -> address
// 	- Renewal list tail(single slot): 'validations-renewal-tail' -> address
// 	- Renewal list prev(Mapping): 'Blake2b(address + 'validations-renewal-prev') -> address
// 	- Renewal list next(Mapping): 'Blake2b(address + 'validations-renewal-next') -> address

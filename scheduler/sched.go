// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package scheduler

// Scheduler defines the interface of schedulers.
type Scheduler interface {
	Schedule(nowTime uint64) (newBlockTime uint64)
	IsTheTime(newBlockTime uint64) bool
	Updates(newBlockTime uint64, totalWeight uint64) (updates []Proposer, score uint64)
}

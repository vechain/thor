// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

type blockStats struct {
	exec, commit, real         mclock.AbsTime
	txs                        int
	usedGas                    uint64
	processed, queued, ignored int
}

func (s *blockStats) UpdateProcessed(n int, txs int, exec, commit, real mclock.AbsTime, usedGas uint64) {
	s.processed += n
	s.txs += txs
	s.exec += exec
	s.commit += commit
	s.real += real
	s.usedGas += usedGas
}

func (s *blockStats) UpdateIgnored(n int) {
	s.ignored += n
}

func (s *blockStats) UpdateQueued(n int) {
	s.queued += n
}

func (s *blockStats) LogContext(last *block.Header) []interface{} {
	return []interface{}{
		"txs", s.txs,
		"mgas", float64(s.usedGas) / 1000 / 1000,
		"et", fmt.Sprintf("%v|%v", common.PrettyDuration(s.exec), common.PrettyDuration(s.commit)),
		"mgas/s", float64(s.usedGas) * 1000 / float64(s.real),
		"id", shortID(last.ID()),
	}
}

func shortID(id thor.Bytes32) string {
	return fmt.Sprintf("[#%v…%x]", block.Number(id), id[28:])
}

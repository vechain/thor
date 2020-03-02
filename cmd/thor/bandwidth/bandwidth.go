// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bandwidth

import (
	"sync"
	"time"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// Bandwidth is gas per second.
type Bandwidth struct {
	value uint64 // gas per second
	lock  sync.Mutex
}

func (b *Bandwidth) Value() uint64 {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.value
}

func (b *Bandwidth) Update(header *block.Header, elapsed time.Duration) (uint64, bool) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if elapsed == 0 {
		return b.value, false
	}

	gasLimit := header.GasLimit()
	gasUsed := header.GasUsed()
	// ignore low gas used
	if gasUsed < gasLimit/10 && gasUsed < thor.MinGasLimit {
		return b.value, false
	}

	// use float64 to avoid overflow
	newValue := uint64(float64(gasUsed) * float64(time.Second) / float64(elapsed))

	if b.value == 0 {
		b.value = newValue
	} else {
		// apply low-pass
		b.value = uint64((float64(b.value)*15 + float64(newValue)) / 16)
	}
	return b.value, true
}

func (b *Bandwidth) SuggestGasLimit() uint64 {
	b.lock.Lock()
	defer b.lock.Unlock()

	// use float64 to avoid overflow
	return uint64(float64(b.value) * float64(thor.TolerableBlockPackingTime) / float64(time.Second))
}

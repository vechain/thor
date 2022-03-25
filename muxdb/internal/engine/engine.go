// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package engine

import (
	"io"

	"github.com/vechain/thor/kv"
)

// Engine defines the interface of K-V engine.
type Engine interface {
	kv.Store
	io.Closer
}

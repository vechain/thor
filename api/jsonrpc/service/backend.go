// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package service

import (
	"github.com/vechain/thor/v2/chain"
)

// Backend holds the thor chain dependencies shared by the JSON-RPC services.
type Backend struct {
	repo *chain.Repository
}

// NewBackend creates a Backend shared by the JSON-RPC services.
func NewBackend(repo *chain.Repository) *Backend {
	return &Backend{repo: repo}
}

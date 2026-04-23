// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package ethview projects native VeChainThor tx / receipt / block types into
// Ethereum-shaped view objects for the eth_* JSON-RPC namespace. It is pure
// and transport-agnostic: callers map the sentinel errors to HTTP / JSON-RPC
// error shapes.
package ethview

import "errors"

// Sentinel errors emitted when a native construct has no faithful eth-shape
// projection. Each maps 1:1 to a data.reason at the transport boundary:
//
//	ErrNotRepresentable              -> "tx_not_representable"
//	ErrBlockContainsNonRepresentable -> "block_contains_tx_not_representable"
var (
	ErrNotRepresentable              = errors.New("tx not representable in eth view")
	ErrBlockContainsNonRepresentable = errors.New("block contains non-representable tx")
)

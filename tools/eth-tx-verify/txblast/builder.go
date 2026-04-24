// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

// Spec describes one (type, clauses, path) combination sent each batch.
// Type is the tx type byte (0x00 legacy / 0x51 VeChain dyn-fee / 0x02 ETH dyn-fee).
// Path is "rest" (POST /transactions) or "rpc" (POST /rpc eth_sendRawTransaction).
type Spec struct {
	Type    byte
	Clauses int
	Path    string
}

// BuildMatrix returns the 10-entry default batch matrix:
// {0x00, 0x51, 0x02} × {1, 3 clauses for 0x00/0x51; 1 only for 0x02} × {rest, rpc}.
// V1 invariant: both submit paths must accept every combination symmetrically.
func BuildMatrix() []Spec {
	return []Spec{
		{Type: 0x00, Clauses: 1, Path: "rest"},
		{Type: 0x00, Clauses: 1, Path: "rpc"},
		{Type: 0x00, Clauses: 3, Path: "rest"},
		{Type: 0x00, Clauses: 3, Path: "rpc"},
		{Type: 0x51, Clauses: 1, Path: "rest"},
		{Type: 0x51, Clauses: 1, Path: "rpc"},
		{Type: 0x51, Clauses: 3, Path: "rest"},
		{Type: 0x51, Clauses: 3, Path: "rpc"},
		{Type: 0x02, Clauses: 1, Path: "rest"},
		{Type: 0x02, Clauses: 1, Path: "rpc"},
	}
}

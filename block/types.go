// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package block

// Vote represents the bft vote in block header.
type Vote uint

const (
	WIT Vote = iota
	COM
)

// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package block

// Vote represents the bft vote in block header.
type Vote bool

const (
	WIT Vote = false
	COM Vote = true
)

func (v *Vote) String() string {
	if v == nil {
		return "N/A"
	}

	if *v == COM {
		return "COM"
	}
	return "WIT"
}

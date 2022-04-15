// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package block

import "errors"

// Vote represents the bft vote in block header.
type Vote uint

const (
	WIT Vote = iota
	COM
)

func TestVote(v Vote) error {
	if v == COM || v == WIT {
		return nil
	}
	return errors.New("invalid BFT vote")
}

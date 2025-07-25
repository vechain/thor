// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package renewal

import "math/big"

type Renewal struct {
	ChangeTVL            *big.Int
	ChangeWeight         *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}

type Exit struct {
	ExitedTVL            *big.Int
	ExitedTVLWeight      *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}

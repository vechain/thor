// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Tale of two dependencies.
// Reason:
// Currently, thor depends on v1.8.14 of go-ethereum project.
// However, Constantinople upgrade requires v1.8.27 go-ethereum dependency.
// Solution:
// This patch exists to temporarily reflect the change of library before
// thor finally upgrades fully to dependency v1.8.27.

package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/v2/thor"
)

// CreateAddress2 creates an ethereum address given the address bytes, initial
// contract code hash and a salt.
// v1.8.27
func CreateAddress2(b common.Address, salt [32]byte, inithash []byte) common.Address {
	return common.BytesToAddress(thor.Keccak256([]byte{0xff}, b.Bytes(), salt[:], inithash).Bytes()[12:])
}

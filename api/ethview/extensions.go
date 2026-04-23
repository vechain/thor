// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethview

import (
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/thor"
)

// accessEntry is the eth-shape access list element. Thor always emits an
// empty access list for 0x02 today; the wire shape still has to match
// Ethereum's expected object-with-storageKeys layout.
type accessEntry struct {
	Address     thor.Address   `json:"address"`
	StorageKeys []thor.Bytes32 `json:"storageKeys"`
}

// nativeClause mirrors the VeChainTx clause layout for the extension view on
// 0x00 / 0x51 single-clause projections. It is NOT used on 0x02 (which has
// exactly one clause flattened into the top-level `to / value / input`).
type nativeClause struct {
	To    *thor.Address `json:"to"`
	Value *hexutil.Big  `json:"value"`
	Data  hexutil.Bytes `json:"data"`
}

// nativeReservedStruct exposes the VeChainTx reserved trailer. Only Features
// is carried on the wire — the Unused reserved slots are rejected at tx-decode
// time so nothing observable can land there.
type nativeReservedStruct struct {
	Features hexutil.Uint64 `json:"features"`
}

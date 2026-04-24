// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/rand/v2"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// soloDevRecipients holds dev accounts #1–#3 as clause recipients.
var soloDevRecipients = [3]thor.Address{
	genesis.DevAccounts()[1].Address,
	genesis.DevAccounts()[2].Address,
	genesis.DevAccounts()[3].Address,
}

// fee constants reused from integration tests.
var (
	maxFeePerGas         = big.NewInt(10_000_000_000_000) // 10 Tera-wei
	maxPriorityFeePerGas = big.NewInt(1_000_000_000)      // 1 Giga-wei
)

func randNonce() uint64 { return rand.Uint64() }

// Build constructs and signs a transaction per the given Spec. For 0x02 the
// nonce is mandatory and sequential (spec 3 invariant); for 0x00/0x51 the
// nonce is randomized internally.
//
// chainTag is a 1-byte tag derived from genesis (last byte of genesis block ID).
// chainID is the 2-byte EIP-155 chainID derived via thor.ChainID(genesis).
// blockRef is an 8-byte reference to a recent header — use 0 for solo dev
// (expiration=720 keeps txs valid for many blocks regardless of blockRef).
func Build(spec Spec, key *ecdsa.PrivateKey, nonce uint64, chainTag byte, chainID uint64, blockRef tx.BlockRef) (*tx.Transaction, error) {
	gasPerClause := uint64(21000)
	gasLimit := gasPerClause*uint64(spec.Clauses) + 5000

	switch spec.Type {
	case 0x00: // tx.TypeLegacy
		b := tx.NewBuilder(tx.TypeLegacy).
			ChainTag(chainTag).
			GasPriceCoef(1).
			Expiration(720).
			Gas(gasLimit).
			Nonce(randNonce()).
			BlockRef(blockRef)
		for i := 0; i < spec.Clauses; i++ {
			b = b.Clause(tx.NewClause(&soloDevRecipients[i]).WithValue(big.NewInt(1)))
		}
		return tx.MustSign(b.Build(), key), nil

	case 0x51: // tx.TypeDynamicFee
		b := tx.NewBuilder(tx.TypeDynamicFee).
			ChainTag(chainTag).
			MaxFeePerGas(maxFeePerGas).
			MaxPriorityFeePerGas(maxPriorityFeePerGas).
			Expiration(720).
			Gas(gasLimit).
			Nonce(randNonce()).
			BlockRef(blockRef)
		for i := 0; i < spec.Clauses; i++ {
			b = b.Clause(tx.NewClause(&soloDevRecipients[i]).WithValue(big.NewInt(1)))
		}
		return tx.MustSign(b.Build(), key), nil

	case 0x02: // tx.TypeEthDynamicFee
		if spec.Clauses != 1 {
			return nil, fmt.Errorf("0x02 requires single clause; got %d", spec.Clauses)
		}
		to := soloDevRecipients[0]
		trx := tx.NewBuilder(tx.TypeEthDynamicFee).
			ChainID(new(big.Int).SetUint64(chainID)).
			EthTo(&to).
			EthValue(big.NewInt(1)).
			MaxFeePerGas(maxFeePerGas).
			MaxPriorityFeePerGas(maxPriorityFeePerGas).
			Gas(gasLimit).
			Nonce(nonce).
			Build()
		return tx.MustSign(trx, key), nil

	default:
		return nil, fmt.Errorf("unknown tx type: 0x%02x", spec.Type)
	}
}

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

// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package tx — Ethereum txData implementations.
//
// eth1559TxData implements the txData interface so that Ethereum EIP-1559 transactions
// can flow through the same block-body encoding, pool, and EVM execution path as
// VeChain-native transactions.
//
// VeChain-specific fields that Ethereum transactions do not carry are stubbed:
//
//	blockRef   = 0          → blockRef.Number()=0 ≤ any block; all schedule checks pass.
//	expiration = MaxUint32  → IsExpired (blockNum > 0 + MaxUint32) is never true.
//	chainTag   = 0          → Ethereum replay protection uses chainID; chain tag validation
//	                          is bypassed in txpool/packer/consensus for Ethereum tx types.
//	dependsOn  = nil        → no dependency on another transaction.
//	reserved   = {}         → no feature flags; not delegated; 65-byte signature expected.

package tx

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

// eth1559EmptyReserved is a package-level zero-value sentinel returned by reserved().
// Avoids a heap allocation on every call since eth1559TxData never sets any reserved flags.
var eth1559EmptyReserved reserved

// eth1559TxData implements txData for EIP-1559 typed Ethereum transactions
// (type byte 0x02; wire format: 0x02 || RLP([chainId, nonce, ...])).
type eth1559TxData struct {
	chainID     uint64
	txNonce     uint64   // "nonce" would conflict with the nonce() interface method
	maxPriority *big.Int // maxPriorityFeePerGas
	maxFee      *big.Int // maxFeePerGas
	gasLimit    uint64
	to          *thor.Address // nil = contract creation
	value       *big.Int
	data        []byte
	accessList  []AccessListEntry // always nil/empty — rejected at engine level
	yParity     uint8
	r, s        *big.Int

	// ethHash = Keccak256(rawBytes). Returned by ethTxHash() and used as tx.ID().
	ethHash thor.Bytes32

	// rawBytes holds the full EIP-1559 wire bytes: 0x02 || rlpBody.
	rawBytes []byte
}

func (d *eth1559TxData) txType() byte             { return TypeEthTyped1559 }
func (d *eth1559TxData) chainTag() byte           { return 0 } // Ethereum txs use chainID; chain tag validation is bypassed
func (d *eth1559TxData) blockRef() uint64         { return 0 }
func (d *eth1559TxData) expiration() uint32       { return math.MaxUint32 }
func (d *eth1559TxData) gas() uint64              { return d.gasLimit }
func (d *eth1559TxData) dependsOn() *thor.Bytes32 { return nil }
func (d *eth1559TxData) nonce() uint64            { return d.txNonce }
func (d *eth1559TxData) reserved() *reserved      { return &eth1559EmptyReserved }
func (d *eth1559TxData) ethTxHash() thor.Bytes32  { return d.ethHash }

func (d *eth1559TxData) clauses() []*Clause {
	return []*Clause{NewClause(d.to).WithValue(d.value).WithData(d.data)}
}

func (d *eth1559TxData) maxFeePerGas() *big.Int         { return new(big.Int).Set(d.maxFee) }
func (d *eth1559TxData) maxPriorityFeePerGas() *big.Int { return new(big.Int).Set(d.maxPriority) }

// signingFields must never be called on an Ethereum tx. Transaction.SigningHash()
// type-asserts to *eth1559TxData and calls computeEthSigningHash() directly, bypassing
// this method entirely. Panicking here turns a silent wrong-hash bug (nil → empty RLP
// list → wrong Keccak256) into an immediate, obvious programming error.
func (d *eth1559TxData) signingFields() []any {
	panic("eth1559TxData: signingFields must not be called; Ethereum txs use computeEthSigningHash()")
}

func (d *eth1559TxData) signature() []byte {
	sig := make([]byte, 65)
	d.r.FillBytes(sig[0:32])
	d.s.FillBytes(sig[32:64])
	sig[64] = d.yParity
	return sig
}

// setSignature must not be called: Ethereum txs are pre-signed; the signature is
// embedded in the wire bytes and cannot be changed after construction.
// Panicking here is intentional — calling setSignature on an Ethereum tx is a programming
// error (WithSignature is only valid for VeChain-native txs). Panicking ensures the bug
// is caught immediately rather than silently accepting a mutation that would be ignored.
// TODO: if callers ever need a softer failure, consider a separate interface method.
func (d *eth1559TxData) setSignature(_ []byte) {
	panic("eth1559TxData: setSignature must not be called; Ethereum txs are pre-signed")
}

func (d *eth1559TxData) copy() txData {
	cpy := &eth1559TxData{
		chainID:     d.chainID,
		txNonce:     d.txNonce,
		maxPriority: new(big.Int).Set(d.maxPriority),
		maxFee:      new(big.Int).Set(d.maxFee),
		gasLimit:    d.gasLimit,
		to:          cloneAddress(d.to),
		value:       new(big.Int).Set(d.value),
		data:        bytes.Clone(d.data),
		yParity:     d.yParity,
		r:           new(big.Int).Set(d.r),
		s:           new(big.Int).Set(d.s),
		ethHash:     d.ethHash,
		rawBytes:    bytes.Clone(d.rawBytes),
	}
	if len(d.accessList) > 0 {
		cpy.accessList = make([]AccessListEntry, len(d.accessList))
		copy(cpy.accessList, d.accessList)
	}
	return cpy
}

// computeEthSigningHash returns the EIP-1559 signing hash:
//
//	Keccak256(0x02 || RLP([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas,
//	                        gasLimit, to, value, data, accessList]))
//
// Uses eth1559SigningBody so the rlp:"nil" tag on To is honoured.
func (d *eth1559TxData) computeEthSigningHash() thor.Bytes32 {
	// TODO: review whether passing internal *big.Int pointers (maxPriority, maxFee, value)
	// directly into eth1559SigningBody is safe long-term. RLP encoding is currently read-only
	// so no mutation occurs, but defensive copies (new(big.Int).Set(...)) would be safer.
	return ethKeccakPrefixedRlpHash(TypeEthTyped1559, &eth1559SigningBody{
		ChainID:              new(big.Int).SetUint64(d.chainID),
		Nonce:                d.txNonce,
		MaxPriorityFeePerGas: d.maxPriority,
		MaxFeePerGas:         d.maxFee,
		GasLimit:             d.gasLimit,
		To:                   d.to,
		Value:                d.value,
		Data:                 d.data,
		AccessList:           d.accessList,
	})
}

// encode writes the rlpBody part (rawBytes[1:], without the 0x02 type byte) to w.
// Transaction.encodeTyped prepends the 0x02 byte, producing the standard EIP-1559
// wire format: [0x02 || rlpBody].
func (d *eth1559TxData) encode(w *bytes.Buffer) error {
	_, err := w.Write(d.rawBytes[1:]) // rawBytes = 0x02 || rlpBody; strip 0x02
	return err
}

// decode parses the rlpBody (without the leading 0x02 byte) from the block body.
// It performs only structural/syntactic parsing: field ranges, signature scalar
// bounds, and chain ID are NOT validated here. Semantic validation is the
// responsibility of the caller — consensus runs it via tr.Origin() (ECDSA
// recovery) and the switch checks in validateBlockBody; the mempool runs the
// full NormalizeEthereumTx pipeline before any tx enters the pool.
func (d *eth1559TxData) decode(input []byte) error {
	var body eth1559Transaction
	if err := rlp.DecodeBytes(input, &body); err != nil {
		return err
	}
	if body.ChainID.BitLen() > 64 {
		return errors.New("eth1559TxData: chainID exceeds uint64 range")
	}
	d.chainID = body.ChainID.Uint64()
	d.txNonce = body.Nonce
	// body is a local struct; its *big.Int and slice fields are freshly allocated by the
	// RLP decoder and transferred here by reference. body goes out of scope after this
	// function returns, so there is no aliasing concern.
	// TODO: revisit if the RLP decoder ever reuses buffers or if body is ever reused.
	d.maxPriority = body.MaxPriorityFeePerGas
	d.maxFee = body.MaxFeePerGas
	d.gasLimit = body.GasLimit
	d.to = body.To
	d.value = body.Value
	d.data = body.Data
	d.accessList = body.AccessList
	d.yParity = body.YParity
	d.r, d.s = body.R, body.S
	// Reconstruct 0x02 || rlpBody for ethHash = Keccak256(fullWireBytes).
	d.rawBytes = make([]byte, 1+len(input))
	d.rawBytes[0] = TypeEthTyped1559
	copy(d.rawBytes[1:], input)
	d.ethHash = thor.Keccak256(d.rawBytes)
	return nil
}

// ---------------------------------------------------------------------------
// Construction from NormalizedEthereumTx
// ---------------------------------------------------------------------------

// NewEthereumTransaction converts a NormalizedEthereumTx — produced by NormalizeEthereumTx —
// into a tx.Transaction suitable for EVM execution and block inclusion.
//
// Ethereum replay protection uses chainID (already validated by NormalizeEthereumTx).
// VeChain chain tag validation is bypassed in txpool/packer/consensus for Ethereum tx types.
func NewEthereumTransaction(norm *NormalizedEthereumTx) *Transaction {
	switch norm.TxType {
	case TypeEthTyped1559:
		return newEth1559Tx(norm)
	default:
		panic(fmt.Sprintf("NewEthereumTransaction: unsupported type 0x%02x", norm.TxType))
	}
}

func newEth1559Tx(norm *NormalizedEthereumTx) *Transaction {
	var d eth1559TxData
	// norm.Raw = 0x02 || rlpBody; decode() expects only the rlpBody.
	if err := d.decode(norm.Raw[1:]); err != nil {
		panic(fmt.Sprintf("newEth1559Tx: unexpected decode failure: %v", err))
	}

	t := &Transaction{body: &d}
	t.cache.id.Store(d.ethHash)
	t.cache.origin.Store(norm.Sender)
	// Block-body size: 0x02 || rlpBody = len(norm.Raw).
	t.cache.size.Store(uint64(len(norm.Raw)))
	return t
}

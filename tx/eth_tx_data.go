// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package tx — Ethereum txData implementations.
//
// ethLegacyTxData and eth1559TxData implement the txData interface so that Ethereum
// transactions can flow through the same block-body encoding, pool, and EVM execution
// path as VeChain-native transactions.
//
// VeChain-specific fields that Ethereum transactions do not carry are stubbed:
//
//	blockRef   = 0          → blockRef.Number()=0 ≤ any block; all schedule checks pass.
//	expiration = MaxUint32  → IsExpired (blockNum > 0 + MaxUint32) is never true.
//	chainTag   = caller-supplied from network genesis (stored at construction time).
//	dependsOn  = nil        → no dependency on another transaction.
//	reserved   = {}         → no feature flags; not delegated; 65-byte signature expected.
//
// TODO: bypass chainTag validation in txpool/packer/consensus for Ethereum tx types
// once pool integration is wired up. Currently, direct runtime invocation (bypassing
// the pool) works without that change.

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

// ethLegacyTxData implements txData for Ethereum legacy (EIP-155) transactions
// (block-body type byte: 0x52; wire format: raw RLP list with no type prefix).
type ethLegacyTxData struct {
	txNonce  uint64 // "nonce" would conflict with the nonce() interface method
	gasPrice *big.Int
	gasLimit uint64
	to       *thor.Address // nil = contract creation
	value    *big.Int
	data     []byte
	v, r, s  *big.Int
	chainID  uint64

	// ethHash = Keccak256(rawBytes). Returned by ethTxHash() and used as tx.ID().
	// See Transaction.ID() for the note on the VeChain txID invariant.
	ethHash thor.Bytes32

	// rawBytes holds the original Ethereum wire bytes (raw RLP list, no 0x52 prefix).
	// encode() writes them verbatim for a byte-perfect block body round-trip.
	rawBytes []byte

	// chainTagVal is the network genesis chainTag stored at construction time.
	// Ethereum replay protection uses chainID (validated by NormalizeEthereumTx);
	// chainTag is a VeChain compatibility stub stored here.
	chainTagVal byte
}

func (d *ethLegacyTxData) txType() byte   { return TypeEthLegacy }
func (d *ethLegacyTxData) chainTag() byte { return d.chainTagVal }

// blockRef returns 0 (block 0). blockRef.Number()=0 ≤ any current block, so all
// pool/packer schedule checks pass without any code changes to those layers.
func (d *ethLegacyTxData) blockRef() uint64 { return 0 }

// expiration returns MaxUint32 so IsExpired (blockNum > 0 + MaxUint32) is never true.
func (d *ethLegacyTxData) expiration() uint32 { return math.MaxUint32 }

// clauses maps the single Ethereum to/value/data to a single VeChain clause.
func (d *ethLegacyTxData) clauses() []*Clause {
	return []*Clause{NewClause(d.to).WithValue(d.value).WithData(d.data)}
}

func (d *ethLegacyTxData) gas() uint64              { return d.gasLimit }
func (d *ethLegacyTxData) dependsOn() *thor.Bytes32 { return nil }
func (d *ethLegacyTxData) nonce() uint64            { return d.txNonce }
func (d *ethLegacyTxData) reserved() *reserved      { return &reserved{} }
func (d *ethLegacyTxData) ethTxHash() thor.Bytes32  { return d.ethHash }

// maxFeePerGas and maxPriorityFeePerGas both return gasPrice, mapping EthLegacy
// into the EIP-1559 effective-price formula used by the runtime:
//
//	min(maxFeePerGas, maxPriorityFeePerGas + baseFee)
//	= min(gasPrice, gasPrice + baseFee)
//	= gasPrice   (since baseFee ≥ 0)
func (d *ethLegacyTxData) maxFeePerGas() *big.Int         { return new(big.Int).Set(d.gasPrice) }
func (d *ethLegacyTxData) maxPriorityFeePerGas() *big.Int { return new(big.Int).Set(d.gasPrice) }

// signingFields is unused for Ethereum tx types: Transaction.SigningHash() type-asserts
// to computeEthSigningHash() instead, which handles the nil-To rlp:"nil" tag correctly.
func (d *ethLegacyTxData) signingFields() []any { return nil }

func (d *ethLegacyTxData) signature() []byte {
	sig := make([]byte, 65)
	d.r.FillBytes(sig[0:32])
	d.s.FillBytes(sig[32:64])
	// yParity = (V − 35) & 1  (EIP-155)
	sig[64] = byte(new(big.Int).Sub(d.v, big.NewInt(35)).Uint64() & 1)
	return sig
}

// setSignature must not be called: Ethereum txs are pre-signed; the signature is
// embedded in the wire bytes and cannot be changed after construction.
func (d *ethLegacyTxData) setSignature(_ []byte) {
	panic("ethLegacyTxData: setSignature must not be called; Ethereum txs are pre-signed")
}

func (d *ethLegacyTxData) copy() txData {
	return &ethLegacyTxData{
		txNonce:     d.txNonce,
		gasPrice:    new(big.Int).Set(d.gasPrice),
		gasLimit:    d.gasLimit,
		to:          cloneAddress(d.to),
		value:       new(big.Int).Set(d.value),
		data:        bytes.Clone(d.data),
		v:           new(big.Int).Set(d.v),
		r:           new(big.Int).Set(d.r),
		s:           new(big.Int).Set(d.s),
		chainID:     d.chainID,
		ethHash:     d.ethHash,
		rawBytes:    bytes.Clone(d.rawBytes),
		chainTagVal: d.chainTagVal,
	}
}

// computeEthSigningHash returns the EIP-155 signing hash:
//
//	Keccak256(RLP([nonce, gasPrice, gasLimit, to, value, data, chainID, 0, 0]))
//
// Uses ethLegacySigningBody (a typed struct) so the rlp:"nil" tag on the To field
// encodes contract-creation txs (nil To) correctly as RLP empty string (0x80).
func (d *ethLegacyTxData) computeEthSigningHash() thor.Bytes32 {
	return ethKeccakRlpHash(&ethLegacySigningBody{
		Nonce:    d.txNonce,
		GasPrice: d.gasPrice,
		GasLimit: d.gasLimit,
		To:       d.to,
		Value:    d.value,
		Data:     d.data,
		ChainID:  new(big.Int).SetUint64(d.chainID),
	})
}

// encode writes the raw Ethereum wire bytes (RLP list, no 0x52 prefix) to w.
// Transaction.encodeTyped prepends the 0x52 block-body marker, producing
// the complete block-body encoding: [0x52 || rawEthBytes].
func (d *ethLegacyTxData) encode(w *bytes.Buffer) error {
	_, err := w.Write(d.rawBytes)
	return err
}

// decode parses raw Ethereum legacy bytes (no 0x52 prefix) as stored in the block body.
// This is a light parse — no chainID re-validation or signature verification, since the
// block producer validated those at submission time via NormalizeEthereumTx.
func (d *ethLegacyTxData) decode(input []byte) error {
	var body ethLegacyTransaction
	if err := rlp.DecodeBytes(input, &body); err != nil {
		return err
	}
	// V ≥ 35 is the EIP-155 invariant enforced at engine ingestion; verify it is still
	// intact so chainID extraction (V−35)>>1 cannot underflow.
	if body.V.Sign() <= 0 || body.V.Cmp(big.NewInt(35)) < 0 {
		return errors.New("ethLegacyTxData: V < 35; not an EIP-155 transaction")
	}
	chainID := new(big.Int).Rsh(new(big.Int).Sub(body.V, big.NewInt(35)), 1)

	d.txNonce = body.Nonce
	d.gasPrice = body.GasPrice
	d.gasLimit = body.GasLimit
	d.to = body.To
	d.value = body.Value
	d.data = body.Data
	d.v, d.r, d.s = body.V, body.R, body.S
	d.chainID = chainID.Uint64()
	d.rawBytes = bytes.Clone(input)
	d.ethHash = thor.Keccak256(input)
	return nil
}

// ---------------------------------------------------------------------------
// eth1559TxData
// ---------------------------------------------------------------------------

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

	// ethHash = Keccak256(rawBytes). See ethLegacyTxData for details.
	ethHash thor.Bytes32

	// rawBytes holds the full EIP-1559 wire bytes: 0x02 || rlpBody.
	rawBytes []byte

	chainTagVal byte
}

func (d *eth1559TxData) txType() byte             { return TypeEthTyped1559 }
func (d *eth1559TxData) chainTag() byte           { return d.chainTagVal }
func (d *eth1559TxData) blockRef() uint64         { return 0 }
func (d *eth1559TxData) expiration() uint32       { return math.MaxUint32 }
func (d *eth1559TxData) gas() uint64              { return d.gasLimit }
func (d *eth1559TxData) dependsOn() *thor.Bytes32 { return nil }
func (d *eth1559TxData) nonce() uint64            { return d.txNonce }
func (d *eth1559TxData) reserved() *reserved      { return &reserved{} }
func (d *eth1559TxData) ethTxHash() thor.Bytes32  { return d.ethHash }

func (d *eth1559TxData) clauses() []*Clause {
	return []*Clause{NewClause(d.to).WithValue(d.value).WithData(d.data)}
}

func (d *eth1559TxData) maxFeePerGas() *big.Int         { return new(big.Int).Set(d.maxFee) }
func (d *eth1559TxData) maxPriorityFeePerGas() *big.Int { return new(big.Int).Set(d.maxPriority) }

// signingFields is unused for Ethereum tx types; see ethLegacyTxData.signingFields.
func (d *eth1559TxData) signingFields() []any { return nil }

func (d *eth1559TxData) signature() []byte {
	sig := make([]byte, 65)
	d.r.FillBytes(sig[0:32])
	d.s.FillBytes(sig[32:64])
	sig[64] = d.yParity
	return sig
}

func (d *eth1559TxData) setSignature(_ []byte) {
	panic("eth1559TxData: setSignature must not be called; Ethereum txs are pre-signed") // TODO should we log here instead ?
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
		chainTagVal: d.chainTagVal,
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
func (d *eth1559TxData) decode(input []byte) error {
	var body eth1559Transaction
	if err := rlp.DecodeBytes(input, &body); err != nil {
		return err
	}
	d.chainID = body.ChainID.Uint64()
	d.txNonce = body.Nonce
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
// chainTag is the last byte of the network genesis block hash. Ethereum replay protection
// uses chainID (already validated by NormalizeEthereumTx); chainTag is stored here as a
// VeChain compatibility stub so the existing pool/packer/consensus checks are satisfied.
// TODO: when pool integration is wired up, add type-checks in txpool/packer/consensus to
// bypass chainTag validation for Ethereum tx types.
func NewEthereumTransaction(norm *NormalizedEthereumTx, chainTag byte) *Transaction {
	switch norm.TxType {
	case TypeEthLegacy:
		return newEthLegacyTx(norm, chainTag)
	case TypeEthTyped1559:
		return newEth1559Tx(norm, chainTag)
	default:
		panic(fmt.Sprintf("NewEthereumTransaction: unsupported type 0x%02x", norm.TxType))
	}
}

func newEthLegacyTx(norm *NormalizedEthereumTx, chainTag byte) *Transaction {
	var d ethLegacyTxData
	// decode() performs a light parse; NormalizeEthereumTx already validated these bytes,
	// so failure here indicates a programming error, not bad user input.
	if err := d.decode(norm.Raw); err != nil {
		panic(fmt.Sprintf("newEthLegacyTx: unexpected decode failure: %v", err))
	}
	d.chainTagVal = chainTag

	t := &Transaction{body: &d}
	// Pre-cache known values to avoid redundant computation.
	t.cache.id.Store(d.ethHash)
	t.cache.origin.Store(norm.Sender)
	// Block-body size: 0x52 marker (1 byte) + raw Ethereum bytes.
	t.cache.size.Store(uint64(1 + len(norm.Raw)))
	return t
}

func newEth1559Tx(norm *NormalizedEthereumTx, chainTag byte) *Transaction {
	var d eth1559TxData
	// norm.Raw = 0x02 || rlpBody; decode() expects only the rlpBody.
	if err := d.decode(norm.Raw[1:]); err != nil {
		panic(fmt.Sprintf("newEth1559Tx: unexpected decode failure: %v", err))
	}
	d.chainTagVal = chainTag

	t := &Transaction{body: &d}
	t.cache.id.Store(d.ethHash)
	t.cache.origin.Store(norm.Sender)
	// Block-body size: 0x02 || rlpBody = len(norm.Raw).
	t.cache.size.Store(uint64(len(norm.Raw)))
	return t
}

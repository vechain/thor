// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/thor"
)

var (
	errIntrinsicGasOverflow = errors.New("intrinsic gas overflow")
	ErrTxTypeNotSupported   = errors.New("transaction type not supported")
	errEmptyTypedTx         = errors.New("empty typed transaction bytes")
)

// Starting from the max value allowed to avoid ambiguity with Ethereum tx type codes.
const (
	LegacyTxType     = 0x00
	DynamicFeeTxType = 0x51
)

// Transaction is an immutable tx type.
type Transaction struct {
	body TxData

	cache struct {
		signingHash  atomic.Value
		origin       atomic.Value
		id           atomic.Value
		unprovedWork atomic.Value
		size         atomic.Value
		intrinsicGas atomic.Value
		hash         atomic.Value
		delegator    atomic.Value
	}
}

// TxData describes details of a tx.
type TxData interface {
	txType() byte
	copy() TxData

	chainTag() byte
	blockRef() uint64
	expiration() uint32
	clauses() []*Clause
	gasPriceCoef() uint8
	gas() uint64
	maxFeePerGas() *big.Int
	maxPriorityFeePerGas() *big.Int
	dependsOn() *thor.Bytes32
	nonce() uint64
	reserved() reserved
	signature() []byte
	setSignature(sig []byte)

	encode(w io.Writer) error
}

// NewTx creates a new transaction.
func NewTx(body TxData) *Transaction {
	tx := new(Transaction)
	tx.setDecoded(body.copy(), 0)
	return tx
}

// Type returns the transaction type.
func (tx *Transaction) Type() uint8 {
	return tx.body.txType()
}

// ChainTag returns chain tag.
func (t *Transaction) ChainTag() byte {
	return t.body.chainTag()
}

// Nonce returns nonce value.
func (t *Transaction) Nonce() uint64 {
	return t.body.nonce()
}

// BlockRef returns block reference, which is first 8 bytes of block hash.
func (t *Transaction) BlockRef() (br BlockRef) {
	binary.BigEndian.PutUint64(br[:], t.body.blockRef())
	return
}

// Expiration returns expiration in unit block.
// A valid transaction requires:
// blockNum in [blockRef.Num... blockRef.Num + Expiration]
func (t *Transaction) Expiration() uint32 {
	return t.body.expiration()
}

// IsExpired returns whether the tx is expired according to the given blockNum.
func (t *Transaction) IsExpired(blockNum uint32) bool {
	return uint64(blockNum) > uint64(t.BlockRef().Number())+uint64(t.body.expiration()) // cast to uint64 to prevent potential overflow
}

// ID returns id of tx.
// ID = hash(signingHash, origin).
// It returns zero Bytes32 if origin not available.
func (t *Transaction) ID() (id thor.Bytes32) {
	if cached := t.cache.id.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { t.cache.id.Store(id) }()

	origin, err := t.Origin()
	if err != nil {
		return
	}
	return thor.Blake2b(t.SigningHash().Bytes(), origin[:])
}

// Hash returns hash of tx.
// Unlike ID, it's the hash of RLP encoded tx.
func (t *Transaction) Hash() (hash thor.Bytes32) {
	if cached := t.cache.hash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { t.cache.hash.Store(hash) }()

	// Legacy tx don't have type prefix.
	if t.Type() == LegacyTxType {
		return rlpHash(t)
	}
	return prefixedRlpHash(t.Type(), t.body)
}

// UnprovedWork returns unproved work of this tx.
// It returns 0, if tx is not signed.
func (t *Transaction) UnprovedWork() (w *big.Int) {
	if cached := t.cache.unprovedWork.Load(); cached != nil {
		return cached.(*big.Int)
	}
	defer func() {
		t.cache.unprovedWork.Store(w)
	}()

	origin, err := t.Origin()
	if err != nil {
		return &big.Int{}
	}
	return t.EvaluateWork(origin)(t.body.nonce())
}

// EvaluateWork try to compute work when tx origin assumed.
func (t *Transaction) EvaluateWork(origin thor.Address) func(nonce uint64) *big.Int {
	var hashWithoutNonce *thor.Bytes32
	switch t.Type() {
	case LegacyTxType:
		hashWithoutNonce = t.hashWithoutNonceLegacyTx(origin)
	case DynamicFeeTxType:
		hashWithoutNonce = t.hashWithoutNonceDynamicFeeTx(origin)
	default:
		panic(ErrTxTypeNotSupported)
	}

	return func(nonce uint64) *big.Int {
		var nonceBytes [8]byte
		binary.BigEndian.PutUint64(nonceBytes[:], nonce)
		hash := thor.Blake2b(hashWithoutNonce[:], nonceBytes[:])
		r := new(big.Int).SetBytes(hash[:])
		return r.Div(math.MaxBig256, r)
	}
}

func (t *Transaction) hashWithoutNonceLegacyTx(origin thor.Address) *thor.Bytes32 {
	b := thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, []interface{}{
			t.body.chainTag(),
			t.body.blockRef(),
			t.body.expiration(),
			t.body.clauses(),
			t.body.gasPriceCoef(),
			t.body.dependsOn(),
			t.body.nonce(),
			t.body.reserved(),
			origin,
		})
	})
	return &b
}

func (t *Transaction) hashWithoutNonceDynamicFeeTx(origin thor.Address) *thor.Bytes32 {
	b := thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, []interface{}{
			t.body.chainTag(),
			t.body.blockRef(),
			t.body.expiration(),
			t.body.clauses(),
			t.body.maxFeePerGas(),
			t.body.maxPriorityFeePerGas(),
			t.body.dependsOn(),
			t.body.nonce(),
			t.body.reserved(),
			origin,
		})
	})
	return &b
}

// SigningHash returns hash of tx excludes signature.
func (t *Transaction) SigningHash() (hash thor.Bytes32) {
	if cached := t.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { t.cache.signingHash.Store(hash) }()

	return thor.Blake2bFn(func(w io.Writer) {
		t.body.encode(w)
	})
}

// Gas returns gas provision for this tx.
func (t *Transaction) Gas() uint64 {
	return t.body.gas()
}

// GasPriceCoef returns gas price coef.
// gas price = bgp + bgp * gpc / 255.
func (t *Transaction) GasPriceCoef() uint8 {
	return t.body.gasPriceCoef()
}

func (t *Transaction) MaxFeePerGas() *big.Int {
	return t.body.maxFeePerGas()
}

func (t *Transaction) MaxPriorityFeePerGas() *big.Int {
	return t.body.maxPriorityFeePerGas()
}

// Clauses returns clauses in tx.
func (t *Transaction) Clauses() []*Clause {
	return append([]*Clause(nil), t.body.clauses()...)
}

// DependsOn returns depended tx hash.
func (t *Transaction) DependsOn() *thor.Bytes32 {
	if t.body.dependsOn() == nil {
		return nil
	}
	cpy := *t.body.dependsOn()
	return &cpy
}

// Signature returns signature.
func (t *Transaction) Signature() []byte {
	return append([]byte(nil), t.body.signature()...)
}

// Features returns features.
func (t *Transaction) Features() Features {
	return t.body.reserved().Features
}

// Origin extract address of tx originator from signature.
func (t *Transaction) Origin() (thor.Address, error) {
	if err := t.validateSignatureLength(); err != nil {
		return thor.Address{}, err
	}

	if cached := t.cache.origin.Load(); cached != nil {
		return cached.(thor.Address), nil
	}

	pub, err := crypto.SigToPub(t.SigningHash().Bytes(), t.body.signature()[:65])
	if err != nil {
		return thor.Address{}, err
	}
	origin := thor.Address(crypto.PubkeyToAddress(*pub))
	t.cache.origin.Store(origin)
	return origin, nil
}

// DelegatorSigningHash returns hash of tx components for delegator to sign, by assuming originator address.
// According to VIP-191, it's identical to tx id.
func (t *Transaction) DelegatorSigningHash(origin thor.Address) (hash thor.Bytes32) {
	return thor.Blake2b(t.SigningHash().Bytes(), origin[:])
}

// Delegator returns delegator address who would like to pay for gas fee.
func (t *Transaction) Delegator() (*thor.Address, error) {
	if err := t.validateSignatureLength(); err != nil {
		return nil, err
	}

	if !t.Features().IsDelegated() {
		return nil, nil
	}

	if cached := t.cache.delegator.Load(); cached != nil {
		addr := cached.(thor.Address)
		return &addr, nil
	}

	origin, err := t.Origin()
	if err != nil {
		return nil, err
	}

	pub, err := crypto.SigToPub(t.DelegatorSigningHash(origin).Bytes(), t.body.signature()[65:])
	if err != nil {
		return nil, err
	}

	delegator := thor.Address(crypto.PubkeyToAddress(*pub))

	t.cache.delegator.Store(delegator)
	return &delegator, nil
}

// WithSignature create a new tx with signature set.
// For delegated tx, sig is joined with signatures of originator and delegator.
func (t *Transaction) WithSignature(sig []byte) *Transaction {
	newTx := Transaction{
		body: t.body.copy(),
	}
	// copy sig
	newTx.body.setSignature(append([]byte(nil), sig...))
	return &newTx
}

// TestFeatures test if the tx is compatible with given supported features.
// An error returned if it is incompatible.
func (t *Transaction) TestFeatures(supported Features) error {
	r := t.body.reserved()
	if r.Features&supported != r.Features {
		return errors.New("unsupported features")
	}

	if len(r.Unused) > 0 {
		return errors.New("unused reserved slot")
	}
	return nil
}

// encodeTyped writes the canonical encoding of a typed transaction to w.
func (t *Transaction) encodeTyped(w *bytes.Buffer) error {
	w.WriteByte(t.Type())
	return rlp.Encode(w, t.body)
}

// MarshalBinary returns the canonical encoding of the transaction.
// For legacy transactions, it returns the RLP encoding. For typed
// transactions, it returns the type and the RLP encoding of the tx.
func (tx *Transaction) MarshalBinary() ([]byte, error) {
	if tx.Type() == LegacyTxType {
		return rlp.EncodeToBytes(tx.body)
	}
	var buf bytes.Buffer
	err := tx.encodeTyped(&buf)
	return buf.Bytes(), err
}

// UnmarshalBinary decodes the canonical encoding of transactions.
// It supports legacy RLP transactions and typed transactions.
func (t *Transaction) UnmarshalBinary(b []byte) error {
	if len(b) > 0 && b[0] > 0x7f {
		// It's a legacy transaction.
		var data LegacyTransaction
		err := rlp.DecodeBytes(b, &data)
		if err != nil {
			return err
		}
		t.setDecoded(&data, len(b))
		return nil
	}
	// It's a typed transaction envelope.
	inner, err := t.decodeTyped(b)
	if err != nil {
		return err
	}
	t.setDecoded(inner, len(b))
	return nil
}

// EncodeRLP implements rlp.Encoder
func (t *Transaction) EncodeRLP(w io.Writer) error {
	if t.Type() == LegacyTxType {
		return rlp.Encode(w, &t.body)
	}
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()

	if err := t.encodeTyped(buf); err != nil {
		return err
	}
	return rlp.Encode(w, buf.Bytes())
}

// DecodeRLP implements rlp.Decoder
func (t *Transaction) DecodeRLP(s *rlp.Stream) error {
	kind, size, err := s.Kind()

	switch {
	case err != nil:
		return err
	case kind == rlp.List:
		// It's a legacy transaction.
		var body LegacyTransaction
		if err := s.Decode(&body); err != nil {
			return err
		}
		*t = Transaction{body: &body}

		t.cache.size.Store(thor.StorageSize(rlp.ListSize(size)))
		return nil
	case kind == rlp.String:
		// It's a typed TX.
		var b []byte
		if b, err = s.Bytes(); err != nil {
			return err
		}
		inner, err := t.decodeTyped(b)
		if err == nil {
			t.setDecoded(inner, len(b))
		}
		return err
	default:
		return rlp.ErrExpectedList
	}
}

// decodeTyped decodes a typed transaction from the canonical format.
func (tx *Transaction) decodeTyped(b []byte) (TxData, error) {
	if len(b) == 0 {
		return nil, errEmptyTypedTx
	}
	switch b[0] {
	case DynamicFeeTxType:
		var body DynamicFeeTransaction
		err := rlp.DecodeBytes(b[1:], &body)
		return &body, err
	default:
		return nil, ErrTxTypeNotSupported
	}
}

// setDecoded sets the inner transaction and size after decoding.
func (t *Transaction) setDecoded(body TxData, size int) {
	t.body = body
	if size > 0 {
		t.cache.size.Store(thor.StorageSize(rlp.ListSize(uint64(size))))
	}
}

// Size returns size in bytes when RLP encoded.
func (t *Transaction) Size() thor.StorageSize {
	if cached := t.cache.size.Load(); cached != nil {
		return cached.(thor.StorageSize)
	}
	var size thor.StorageSize
	rlp.Encode(&size, t)
	t.cache.size.Store(size)
	return size
}

// IntrinsicGas returns intrinsic gas of tx.
func (t *Transaction) IntrinsicGas() (uint64, error) {
	if cached := t.cache.intrinsicGas.Load(); cached != nil {
		return cached.(uint64), nil
	}

	gas, err := IntrinsicGas(t.body.clauses()...)
	if err != nil {
		return 0, err
	}
	t.cache.intrinsicGas.Store(gas)
	return gas, nil
}

// GasPrice returns gas price.
// gasPrice = baseGasPrice + baseGasPrice * gasPriceCoef / 255
func (t *Transaction) GasPrice(baseGasPrice *big.Int) *big.Int {
	x := new(big.Int).Set(t.body.maxFeePerGas())
	x.Mul(x, baseGasPrice)
	x.Div(x, big.NewInt(math.MaxUint8))
	return x.Add(x, baseGasPrice)
}

// ProvedWork returns proved work.
// Unproved work will be considered as proved work if block ref is do the prefix of a block's ID,
// and tx delay is less equal to MaxTxWorkDelay.
func (t *Transaction) ProvedWork(headBlockNum uint32, getBlockID func(uint32) (thor.Bytes32, error)) (*big.Int, error) {
	ref := t.BlockRef()
	refNum := ref.Number()
	if refNum >= headBlockNum {
		return &big.Int{}, nil
	}

	if delay := headBlockNum - refNum; delay > thor.MaxTxWorkDelay {
		return &big.Int{}, nil
	}

	id, err := getBlockID(refNum)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(id[:], ref[:]) {
		return t.UnprovedWork(), nil
	}
	return &big.Int{}, nil
}

// OverallGasPrice calculate overall gas price.
// overallGasPrice = gasPrice + baseGasPrice * wgas/gas.
func (t *Transaction) OverallGasPrice(baseGasPrice *big.Int, provedWork *big.Int) *big.Int {
	gasPrice := t.GasPrice(baseGasPrice)

	if provedWork.Sign() == 0 {
		return gasPrice
	}

	wgas := workToGas(provedWork, t.BlockRef().Number())
	if wgas == 0 {
		return gasPrice
	}
	if wgas > t.body.gas() {
		wgas = t.body.gas()
	}

	x := new(big.Int).SetUint64(wgas)
	x.Mul(x, baseGasPrice)
	x.Div(x, new(big.Int).SetUint64(t.body.gas()))
	return x.Add(x, gasPrice)
}

func (t *Transaction) String() string {
	var (
		originStr    = "N/A"
		br           BlockRef
		dependsOn    = "nil"
		delegatorStr = "N/A"
	)
	if origin, err := t.Origin(); err == nil {
		originStr = origin.String()
	}
	if delegator, _ := t.Delegator(); delegator != nil {
		delegatorStr = delegator.String()
	}

	binary.BigEndian.PutUint64(br[:], t.body.blockRef())
	if t.body.dependsOn() != nil {
		dependsOn = t.body.dependsOn().String()
	}

	s := fmt.Sprintf(`
	Tx(%v, %v)
	Origin:         %v
	Clauses:        %v
	Gas:            %v
	ChainTag:       %v
	BlockRef:       %v-%x
	Expiration:     %v
	DependsOn:      %v
	Nonce:          %v
	UnprovedWork:   %v
	Delegator:      %v
	Signature:      0x%x
`, t.ID(), t.Size(), originStr, t.body.clauses(), t.body.gas(),
		t.body.chainTag(), br.Number(), br[4:], t.body.expiration(), dependsOn, t.body.nonce(), t.UnprovedWork(), delegatorStr, t.body.signature())

	if t.Type() == LegacyTxType {
		return fmt.Sprintf(`%v
		GasPriceCoef:   %v
		`, s, t.body.gasPriceCoef())
	}

	return fmt.Sprintf(`%v
		MaxFeePerGas:   %v
		MaxPriorityFeePerGas: %v
		`, s, t.body.maxFeePerGas(), t.body.maxPriorityFeePerGas())
}

func (t *Transaction) validateSignatureLength() error {
	expectedSigLen := 65
	if t.Features().IsDelegated() {
		expectedSigLen *= 2
	}

	if len(t.body.signature()) != expectedSigLen {
		return secp256k1.ErrInvalidSignatureLen
	}
	return nil
}

// IntrinsicGas calculate intrinsic gas cost for tx with such clauses.
func IntrinsicGas(clauses ...*Clause) (uint64, error) {
	if len(clauses) == 0 {
		return thor.TxGas + thor.ClauseGas, nil
	}

	var total = thor.TxGas
	var overflow bool
	for _, c := range clauses {
		gas, err := dataGas(c.body.Data)
		if err != nil {
			return 0, err
		}
		total, overflow = math.SafeAdd(total, gas)
		if overflow {
			return 0, errIntrinsicGasOverflow
		}

		var cgas uint64
		if c.IsCreatingContract() {
			// contract creation
			cgas = thor.ClauseGasContractCreation
		} else {
			cgas = thor.ClauseGas
		}

		total, overflow = math.SafeAdd(total, cgas)
		if overflow {
			return 0, errIntrinsicGasOverflow
		}
	}
	return total, nil
}

// see core.IntrinsicGas
func dataGas(data []byte) (uint64, error) {
	if len(data) == 0 {
		return 0, nil
	}
	var z, nz uint64
	for _, byt := range data {
		if byt == 0 {
			z++
		} else {
			nz++
		}
	}
	zgas, overflow := math.SafeMul(params.TxDataZeroGas, z)
	if overflow {
		return 0, errIntrinsicGasOverflow
	}
	nzgas, overflow := math.SafeMul(params.TxDataNonZeroGas, nz)
	if overflow {
		return 0, errIntrinsicGasOverflow
	}

	gas, overflow := math.SafeAdd(zgas, nzgas)
	if overflow {
		return 0, errIntrinsicGasOverflow
	}
	return gas, nil
}

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
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/thor"
)

var (
	errIntrinsicGasOverflow = errors.New("intrinsic gas overflow")
)

// Transaction is an immutable tx type.
type Transaction struct {
	body body

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

// body describes details of a tx.
type body struct {
	ChainTag     byte
	BlockRef     uint64
	Expiration   uint32
	Clauses      []*Clause
	GasPriceCoef uint8
	Gas          uint64
	DependsOn    *thor.Bytes32 `rlp:"nil"`
	Nonce        uint64
	Reserved     reserved
	Signature    []byte
}

// ChainTag returns chain tag.
func (t *Transaction) ChainTag() byte {
	return t.body.ChainTag
}

// Nonce returns nonce value.
func (t *Transaction) Nonce() uint64 {
	return t.body.Nonce
}

// BlockRef returns block reference, which is first 8 bytes of block hash.
func (t *Transaction) BlockRef() (br BlockRef) {
	binary.BigEndian.PutUint64(br[:], t.body.BlockRef)
	return
}

// Expiration returns expiration in unit block.
// A valid transaction requires:
// blockNum in [blockRef.Num... blockRef.Num + Expiration]
func (t *Transaction) Expiration() uint32 {
	return t.body.Expiration
}

// IsExpired returns whether the tx is expired according to the given blockNum.
func (t *Transaction) IsExpired(blockNum uint32) bool {
	return uint64(blockNum) > uint64(t.BlockRef().Number())+uint64(t.body.Expiration) // cast to uint64 to prevent potential overflow
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
	return thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, t)
	})
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
	return t.EvaluateWork(origin)(t.body.Nonce)
}

// EvaluateWork try to compute work when tx origin assumed.
func (t *Transaction) EvaluateWork(origin thor.Address) func(nonce uint64) *big.Int {
	hashWithoutNonce := thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, []interface{}{
			t.body.ChainTag,
			t.body.BlockRef,
			t.body.Expiration,
			t.body.Clauses,
			t.body.GasPriceCoef,
			t.body.Gas,
			t.body.DependsOn,
			&t.body.Reserved,
			origin,
		})
	})

	return func(nonce uint64) *big.Int {
		var nonceBytes [8]byte
		binary.BigEndian.PutUint64(nonceBytes[:], nonce)
		hash := thor.Blake2b(hashWithoutNonce[:], nonceBytes[:])
		r := new(big.Int).SetBytes(hash[:])
		return r.Div(math.MaxBig256, r)
	}
}

// SigningHash returns hash of tx excludes signature.
func (t *Transaction) SigningHash() (hash thor.Bytes32) {
	if cached := t.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { t.cache.signingHash.Store(hash) }()

	return thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, []interface{}{
			t.body.ChainTag,
			t.body.BlockRef,
			t.body.Expiration,
			t.body.Clauses,
			t.body.GasPriceCoef,
			t.body.Gas,
			t.body.DependsOn,
			t.body.Nonce,
			&t.body.Reserved,
		})
	})
}

// GasPriceCoef returns gas price coef.
// gas price = bgp + bgp * gpc / 255.
func (t *Transaction) GasPriceCoef() uint8 {
	return t.body.GasPriceCoef
}

// Gas returns gas provision for this tx.
func (t *Transaction) Gas() uint64 {
	return t.body.Gas
}

// Clauses returns caluses in tx.
func (t *Transaction) Clauses() []*Clause {
	return append([]*Clause(nil), t.body.Clauses...)
}

// DependsOn returns depended tx hash.
func (t *Transaction) DependsOn() *thor.Bytes32 {
	if t.body.DependsOn == nil {
		return nil
	}
	cpy := *t.body.DependsOn
	return &cpy
}

// Signature returns signature.
func (t *Transaction) Signature() []byte {
	return append([]byte(nil), t.body.Signature...)
}

// Features returns features.
func (t *Transaction) Features() Features {
	return t.body.Reserved.Features
}

// Origin extract address of tx originator from signature.
func (t *Transaction) Origin() (thor.Address, error) {
	if err := t.validateSignatureLength(); err != nil {
		return thor.Address{}, err
	}

	if cached := t.cache.origin.Load(); cached != nil {
		return cached.(thor.Address), nil
	}

	pub, err := crypto.SigToPub(t.SigningHash().Bytes(), t.body.Signature[:65])
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

	pub, err := crypto.SigToPub(t.DelegatorSigningHash(origin).Bytes(), t.body.Signature[65:])
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
		body: t.body,
	}
	// copy sig
	newTx.body.Signature = append([]byte(nil), sig...)
	return &newTx
}

// TestFeatures test if the tx is compatible with given supported features.
// An error returned if it is incompatible.
func (t *Transaction) TestFeatures(supported Features) error {
	r := &t.body.Reserved
	if r.Features&supported != r.Features {
		return errors.New("unsupported features")
	}

	if len(r.Unused) > 0 {
		return errors.New("unused reserved slot")
	}
	return nil
}

// EncodeRLP implements rlp.Encoder
func (t *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &t.body)
}

// DecodeRLP implements rlp.Decoder
func (t *Transaction) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	var body body
	if err := s.Decode(&body); err != nil {
		return err
	}
	*t = Transaction{body: body}

	t.cache.size.Store(metric.StorageSize(rlp.ListSize(size)))
	return nil
}

// Size returns size in bytes when RLP encoded.
func (t *Transaction) Size() metric.StorageSize {
	if cached := t.cache.size.Load(); cached != nil {
		return cached.(metric.StorageSize)
	}
	var size metric.StorageSize
	rlp.Encode(&size, t)
	t.cache.size.Store(size)
	return size
}

// IntrinsicGas returns intrinsic gas of tx.
func (t *Transaction) IntrinsicGas() (uint64, error) {
	if cached := t.cache.intrinsicGas.Load(); cached != nil {
		return cached.(uint64), nil
	}

	gas, err := IntrinsicGas(t.body.Clauses...)
	if err != nil {
		return 0, err
	}
	t.cache.intrinsicGas.Store(gas)
	return gas, nil
}

// GasPrice returns gas price.
// gasPrice = baseGasPrice + baseGasPrice * gasPriceCoef / 255
func (t *Transaction) GasPrice(baseGasPrice *big.Int) *big.Int {
	x := big.NewInt(int64(t.body.GasPriceCoef))
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
	if wgas > t.body.Gas {
		wgas = t.body.Gas
	}

	x := new(big.Int).SetUint64(wgas)
	x.Mul(x, baseGasPrice)
	x.Div(x, new(big.Int).SetUint64(t.body.Gas))
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

	binary.BigEndian.PutUint64(br[:], t.body.BlockRef)
	if t.body.DependsOn != nil {
		dependsOn = t.body.DependsOn.String()
	}

	return fmt.Sprintf(`
	Tx(%v, %v)
	Origin:         %v
	Clauses:        %v
	GasPriceCoef:   %v
	Gas:            %v
	ChainTag:       %v
	BlockRef:       %v-%x
	Expiration:     %v
	DependsOn:      %v
	Nonce:          %v
	UnprovedWork:   %v
	Delegator:      %v
	Signature:      0x%x
`, t.ID(), t.Size(), originStr, t.body.Clauses, t.body.GasPriceCoef, t.body.Gas,
		t.body.ChainTag, br.Number(), br[4:], t.body.Expiration, dependsOn, t.body.Nonce, t.UnprovedWork(), delegatorStr, t.body.Signature)
}

func (t *Transaction) validateSignatureLength() error {
	expectedSigLen := 65
	if t.Features().IsDelegated() {
		expectedSigLen *= 2
	}

	if len(t.body.Signature) != expectedSigLen {
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

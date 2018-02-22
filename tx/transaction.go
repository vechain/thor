package tx

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/thor"
)

const signerCacheSize = 1024

var (
	signerCache = cache.NewLRU(signerCacheSize)
	maxUint256  = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
	invalidTxID = func() (h thor.Hash) {
		for i := range h {
			h[i] = 0xff
		}
		return
	}()
)

// Transaction is an immutable tx type.
type Transaction struct {
	body body

	cache struct {
		signingHash *thor.Hash
		signer      *thor.Address
		id          *thor.Hash
	}
}

// body describes details of a tx.
type body struct {
	ChainTag     byte
	BlockRef     uint64
	Clauses      []*Clause
	GasPrice     *big.Int
	Gas          uint64
	Nonce        uint64
	DependsOn    *thor.Hash `rlp:"nil"`
	ReservedBits uint32
	Signature    []byte
}

// ChainTag returns chain tag.
func (t *Transaction) ChainTag() byte {
	return t.body.ChainTag
}

// BlockRef returns block reference, which is first 8 bytes of block hash.
func (t *Transaction) BlockRef() (br BlockRef) {
	binary.BigEndian.PutUint64(br[:], t.body.BlockRef)
	return
}

// ID returns id of tx.
// ID = hash(signingHash, signer).
// It returns invalidTxID if signer not available.
func (t *Transaction) ID() (id thor.Hash) {
	if cached := t.cache.id; cached != nil {
		return *cached
	}
	defer func() { t.cache.id = &id }()

	signer, err := t.Signer()
	if err != nil {
		return invalidTxID
	}
	return t.makeID(signer)
}

func (t *Transaction) makeID(signer thor.Address) (id thor.Hash) {
	hw := sha3.NewKeccak256()
	hw.Write(t.SigningHash().Bytes())
	hw.Write(signer.Bytes())
	hw.Sum(id[:0])
	return
}

func idToWork(id thor.Hash) *big.Int {
	result := new(big.Int).SetBytes(id[:])
	return result.Div(maxUint256, result)
}

// ProvedWork returns proved work of this tx.
// It returns 0, if tx is not signed.
func (t *Transaction) ProvedWork() *big.Int {
	_, err := t.Signer()
	if err != nil {
		return &big.Int{}
	}
	return idToWork(t.ID())
}

// EvaluateWork try to compute work when tx signer assumed.
func (t *Transaction) EvaluateWork(signer thor.Address) *big.Int {
	result := new(big.Int).SetBytes(t.makeID(signer).Bytes())
	return result.Div(maxUint256, result)
}

// SigningHash returns hash of tx excludes signature.
func (t *Transaction) SigningHash() (hash thor.Hash) {
	if cached := t.cache.signingHash; cached != nil {
		return *cached
	}
	defer func() { t.cache.signingHash = &hash }()

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, []interface{}{
		t.body.ChainTag,
		t.body.BlockRef,
		t.body.Clauses,
		t.body.GasPrice,
		t.body.Gas,
		t.body.Nonce,
		t.body.DependsOn,
		t.body.ReservedBits,
	})
	hw.Sum(hash[:0])
	return
}

// GasPrice returns gas price.
func (t *Transaction) GasPrice() *big.Int {
	return new(big.Int).Set(t.body.GasPrice)
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
func (t *Transaction) DependsOn() *thor.Hash {
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

// Signer extract signer of tx from signature.
func (t *Transaction) Signer() (signer thor.Address, err error) {
	if cached := t.cache.signer; cached != nil {
		return *t.cache.signer, nil
	}
	defer func() {
		if err == nil {
			t.cache.signer = &signer
		}
	}()

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, &t)
	var hash thor.Hash
	hw.Sum(hash[:0])

	if v, ok := signerCache.Get(hash); ok {
		signer = v.(thor.Address)
		return
	}
	defer func() {
		if err == nil {
			signerCache.Add(hash, signer)
		}
	}()
	pub, err := crypto.SigToPub(t.SigningHash().Bytes(), t.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}
	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// WithSignature create a new tx with signature set.
func (t *Transaction) WithSignature(sig []byte) *Transaction {
	newTx := Transaction{
		body: t.body,
	}
	// copy sig
	newTx.body.Signature = append([]byte(nil), sig...)
	return &newTx
}

// ReservedBits returns reserved bits for backward compatibility purpose.
func (t *Transaction) ReservedBits() uint32 {
	return t.body.ReservedBits
}

// EncodeRLP implements rlp.Encoder
func (t *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &t.body)
}

// DecodeRLP implements rlp.Decoder
func (t *Transaction) DecodeRLP(s *rlp.Stream) error {
	var body body
	if err := s.Decode(&body); err != nil {
		return err
	}
	*t = Transaction{
		body: body,
	}
	return nil
}

// IntrinsicGas returns intrinsic gas of tx.
// That's sum of all clauses intrinsic gas.
func (t *Transaction) IntrinsicGas() (uint64, error) {
	clauseCount := len(t.body.Clauses)
	if clauseCount == 0 {
		return params.TxGas, nil
	}

	firstClause := t.body.Clauses[0]
	total := core.IntrinsicGas(firstClause.body.Data, firstClause.body.To == nil, true)

	for _, c := range t.body.Clauses[1:] {
		contractCreation := c.body.To == nil
		total.Add(total, core.IntrinsicGas(c.body.Data, contractCreation, true))

		// sub over-payed gas for clauses after the first one.
		if contractCreation {
			total.Sub(total, new(big.Int).SetUint64(params.TxGasContractCreation-thor.ClauseGasContractCreation))
		} else {
			total.Sub(total, new(big.Int).SetUint64(params.TxGas-thor.ClauseGas))
		}
	}

	if total.BitLen() > 64 {
		return 0, errors.New("intrinsic gas too large")
	}
	return total.Uint64(), nil
}

func (t *Transaction) String() string {
	var (
		from      string
		br        BlockRef
		dependsOn string
	)
	signer, err := t.Signer()
	if err != nil {
		from = "N/A"
	} else {
		from = signer.String()
	}

	binary.BigEndian.PutUint64(br[:], t.body.BlockRef)
	if t.body.DependsOn == nil {
		dependsOn = "nil"
	} else {
		dependsOn = t.body.DependsOn.String()
	}

	return fmt.Sprintf(`
	Tx(%v)
	From:			%v
	Clauses:		%v
	GasPrice:		%v
	Gas:			%v
	ChainTag:		%v
	BlockRef:		%v-%x
	DependsOn:		%v
	ReservedBits:	%v
	Signature:		0x%x
`, t.ID(), from, t.body.Clauses, t.body.GasPrice, t.body.Gas,
		t.body.ChainTag, br.Number(), br[4:], dependsOn, t.body.ReservedBits, t.body.Signature)
}

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
	signerCache        = cache.NewLRU(signerCacheSize)
	maxUint256         = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
	errGenesisIDNotSet = errors.New("tx genesis id not set")

	invalidTxID = func() (h thor.Hash) {
		for i := range h {
			h[i] = 0xff
		}
		return
	}()
)

// Transaction is an immutable tx type.
type Transaction struct {
	body      body
	genesisID thor.Hash
	cache     struct {
		signingHash *thor.Hash
		signer      *thor.Address
		id          *thor.Hash
	}
}

// body describes details of a tx.
type body struct {
	Clauses   []*Clause
	GasPrice  *big.Int
	Gas       uint64
	Nonce     uint64
	BlockRef  uint64
	DependsOn *thor.Hash `rlp:"nil"`
	Reserved  uint64
	Signature []byte
}

// Reserved returns reserved value.
// Must be 0 now.
func (t *Transaction) Reserved() uint64 {
	return t.body.Reserved
}

// ID returns id of tx.
// ID = hash(signingHash, signer).
// It returns empty hash if signer not available.
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
	signingHash, err := t.SigningHash()
	if err != nil {
		return invalidTxID
	}
	hw := sha3.NewKeccak256()
	hw.Write(signingHash[:])
	hw.Write(signer.Bytes())
	hw.Sum(id[:0])
	return
}

// ProvedWork returns proved work of this tx.
// It returns 0, if tx is not signed.
func (t *Transaction) ProvedWork() *big.Int {
	if id := t.ID(); id != invalidTxID {
		return idToWork(id)
	}
	return &big.Int{}
}

func idToWork(id thor.Hash) *big.Int {
	result := new(big.Int).SetBytes(id[:])
	return result.Div(maxUint256, result)
}

// EvaluateWork try to compute work when signer assumed.
func (t *Transaction) EvaluateWork(signer thor.Address) *big.Int {
	if id := t.makeID(signer); id != invalidTxID {
		return idToWork(id)
	}
	return &big.Int{}
}

// SigningHash returns hash of tx excludes signature.
// error returned if genesis hash not set.
func (t *Transaction) SigningHash() (hash thor.Hash, err error) {
	if (thor.Hash{}) == t.genesisID {
		return thor.Hash{}, errGenesisIDNotSet
	}

	if cached := t.cache.signingHash; cached != nil {
		return *cached, nil
	}

	defer func() { t.cache.signingHash = &hash }()

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, []interface{}{
		t.body.Clauses,
		t.body.GasPrice,
		t.body.Gas,
		t.body.Nonce,
		t.body.BlockRef,
		t.body.DependsOn,
		t.body.Reserved,

		t.genesisID,
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

// BlockRef returns block reference, which is first 8 bytes of block hash.
func (t *Transaction) BlockRef() (br BlockRef) {
	binary.BigEndian.PutUint64(br[:], t.body.BlockRef)
	return
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
	if (thor.Hash{}) == t.genesisID {
		return thor.Address{}, errGenesisIDNotSet
	}

	if cached := t.cache.signer; cached != nil {
		return *t.cache.signer, nil
	}

	defer func() {
		if err == nil {
			t.cache.signer = &signer
		}
	}()

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, []interface{}{
		&t.body,
		t.genesisID,
	})
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
	signingHash, err := t.SigningHash()
	if err != nil {
		return thor.Address{}, err
	}
	pub, err := crypto.SigToPub(signingHash[:], t.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}
	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// WithGenesisID create a new tx with genesis id set.
func (t *Transaction) WithGenesisID(genesisID thor.Hash) *Transaction {
	newTx := Transaction{
		body:      t.body,
		genesisID: genesisID,
	}
	return &newTx
}

// WithSignature create a new tx with signature set.
func (t *Transaction) WithSignature(sig []byte) *Transaction {
	newTx := Transaction{
		body:      t.body,
		genesisID: t.genesisID,
	}
	// copy sig
	newTx.body.Signature = append([]byte(nil), sig...)
	return &newTx
}

// EncodeRLP implements rlp.Encoder
func (t *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &t.body)
}

// DecodeRLP implements rlp.Decoder
func (t *Transaction) DecodeRLP(s *rlp.Stream) error {
	var tx Transaction
	if err := s.Decode(&tx.body); err != nil {
		return err
	}
	*t = tx
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
	From:		%v	
	Clauses:	%v
	GasPrice:	%v
	Gas:		%v
	BlockRef:	%v-%x
	DependsOn:	%v
	Reserved:	%v
	Signature:	0x%x
`, t.ID(), from, t.body.Clauses, t.body.GasPrice,
		t.body.Gas, br.Number(), br[4:], dependsOn, t.body.Reserved, t.body.Signature)
}

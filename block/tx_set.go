package block

import (
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// TxSet transaction set
type TxSet struct {
	body txSetBody

	cache struct {
		root        atomic.Value
		signer      atomic.Value
		signingHash atomic.Value
	}
}

type txSetBody struct {
	Txs        tx.Transactions
	Timestamp  uint64
	TotalScore uint64
	Signature  []byte
}

// NewTxSet creates an instance of TxSet
func NewTxSet(txs tx.Transactions, timestamp, totalScore uint64) *TxSet {
	return &TxSet{
		body: txSetBody{
			Txs:        txs,
			Timestamp:  timestamp,
			TotalScore: totalScore,
		},
	}
}

// Signer returns the signer
func (ts *TxSet) Signer() (signer thor.Address, err error) {
	if cached := ts.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() {
		if err == nil {
			ts.cache.signer.Store(signer)
		}
	}()

	pub, err := crypto.SigToPub(ts.SigningHash().Bytes(), ts.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// SigningHash computes the hash to be signed
func (ts *TxSet) SigningHash() (hash thor.Bytes32) {
	if cached := ts.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { ts.cache.signingHash.Store(hash) }()

	if len(ts.body.Txs) == 0 {
		return thor.Bytes32{}
	}

	hw := thor.NewBlake2b()
	rlp.Encode(hw, []interface{}{ts.body.Txs, ts.body.Timestamp})
	hw.Sum(hash[:0])
	return
}

// WithSignature create a new TxSet object with signature set.
func (ts *TxSet) WithSignature(sig []byte) *TxSet {
	cpy := TxSet{body: ts.body}
	cpy.body.Signature = append([]byte(nil), sig...)
	return &cpy
}

// EncodeRLP implements rlp.Encoder
func (ts *TxSet) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, ts.body)
}

// DecodeRLP implements rlp.Decoder.
func (ts *TxSet) DecodeRLP(s *rlp.Stream) error {
	var body txSetBody

	if err := s.Decode(&body); err != nil {
		return err
	}

	*ts = TxSet{body: body}
	return nil
}

// TxRoot computes the root of the Merkle tree constructed
// from the transactions
func (ts *TxSet) TxsRoot() (root thor.Bytes32) {
	if cached := ts.cache.root.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { ts.cache.root.Store(root) }()

	root = ts.body.Txs.RootHash()
	return
}

// Transactions returns transactions
func (ts *TxSet) Transactions() tx.Transactions {
	return ts.body.Txs
}

// Timestamp returns timestamp
func (ts *TxSet) Timestamp() uint64 {
	return ts.body.Timestamp
}

// TotalScore returns total score
func (ts *TxSet) TotalScore() uint64 {
	return ts.body.TotalScore
}

func (ts *TxSet) IsEmpty() bool {
	return len(ts.body.Txs) == 0
}

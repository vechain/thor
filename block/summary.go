package block

import (
	"bytes"
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// Summary block summary
type Summary struct {
	body summaryBody

	cache struct {
		signingHash atomic.Value
		signer      atomic.Value
		endorseHash atomic.Value
	}
}

type summaryBody struct {
	ParentID  thor.Bytes32
	TxRoot    thor.Bytes32
	Timestamp uint64

	Signature []byte
}

// NewBlockSummary creates a block summary without signature
func NewBlockSummary(parentID, txRoot thor.Bytes32, timestamp uint64) *Summary {
	return &Summary{
		body: summaryBody{
			ParentID:  parentID,
			TxRoot:    txRoot,
			Timestamp: timestamp,
		},
	}
}

// Copy copies the current block summary
func (bs *Summary) Copy() *Summary {
	return &Summary{body: bs.body}
}

// Signer returns the signer
func (bs *Summary) Signer() (signer thor.Address, err error) {
	if cached := bs.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() {
		if err == nil {
			bs.cache.signer.Store(signer)
		}
	}()

	pub, err := crypto.SigToPub(bs.SigningHash().Bytes(), bs.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// SigningHash computes the hash to be signed
func (bs *Summary) SigningHash() (hash thor.Bytes32) {
	if cached := bs.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { bs.cache.signingHash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, []interface{}{
		bs.body.ParentID,
		bs.body.TxRoot,
		bs.body.Timestamp,
	})
	hw.Sum(hash[:0])
	return
}

// WithSignature create a new Summary object with signature set.
func (bs *Summary) WithSignature(sig []byte) *Summary {
	cpy := Summary{body: bs.body}
	cpy.body.Signature = append([]byte(nil), sig...)
	return &cpy
}

// EncodeRLP implements rlp.Encoder
func (bs *Summary) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, bs.body)
}

// DecodeRLP implements rlp.Decoder.
func (bs *Summary) DecodeRLP(s *rlp.Stream) error {
	var body summaryBody

	if err := s.Decode(&body); err != nil {
		return err
	}

	*bs = Summary{body: body}
	return nil
}

// IsEqual ...
func (bs *Summary) IsEqual(_bs *Summary) bool {
	var buff, _buff bytes.Buffer

	bs.EncodeRLP(&buff)
	_bs.EncodeRLP(&_buff)

	return bytes.Compare(buff.Bytes(), _buff.Bytes()) == 0
}

// ParentID returns paraent ID
func (bs *Summary) ParentID() thor.Bytes32 {
	return bs.body.ParentID
}

// Timestamp returns timestamp
func (bs *Summary) Timestamp() uint64 {
	return bs.body.Timestamp
}

// Signature return signature
func (bs *Summary) Signature() []byte {
	return bs.body.Signature
}

// EndorseHash computes the hash for committee member to sign
func (bs *Summary) EndorseHash() (hash thor.Bytes32) {
	if cached := bs.cache.endorseHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { bs.cache.endorseHash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, bs.body)
	hw.Sum(hash[:0])
	return
}

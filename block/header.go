package block

import (
	"encoding/binary"
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/dsa"
)

// Header contains almost all information about a block, except txs.
// It's immutable.
type Header struct {
	subject   subject
	signature []byte

	cache struct {
		hash   *cry.Hash
		signer *acc.Address
	}
}

// subject header excludes signature.
type subject struct {
	ParentHash  cry.Hash
	Timestamp   uint64
	GasLimit    *big.Int
	GasUsed     *big.Int
	Beneficiary acc.Address

	TxsRoot      cry.Hash
	StateRoot    cry.Hash
	ReceiptsRoot cry.Hash
}

// ParentHash returns hash of parent block.
func (h *Header) ParentHash() cry.Hash {
	return h.subject.ParentHash
}

// Number returns sequential number of this block.
func (h *Header) Number() uint32 {
	// inferred from parent hash
	return blockHash(h.subject.ParentHash).blockNumber() + 1
}

// Timestamp returns timestamp of this block.
func (h *Header) Timestamp() uint64 {
	return h.subject.Timestamp
}

// GasLimit returns gas limit of this block.
func (h *Header) GasLimit() *big.Int {
	if h.subject.GasLimit == nil {
		return &big.Int{}
	}
	return new(big.Int).Set(h.subject.GasLimit)
}

// GasUsed returns gas used by txs.
func (h *Header) GasUsed() *big.Int {
	if h.subject.GasUsed == nil {
		return &big.Int{}
	}
	return new(big.Int).Set(h.subject.GasUsed)
}

// Beneficiary returns reward recipient.
func (h *Header) Beneficiary() acc.Address {
	return h.subject.Beneficiary
}

// TxsRoot returns merkle root of txs contained in this block.
func (h *Header) TxsRoot() cry.Hash {
	return h.subject.TxsRoot
}

// StateRoot returns account state merkle root just afert this block being applied.
func (h *Header) StateRoot() cry.Hash {
	return h.subject.StateRoot
}

// ReceiptsRoot returns merkle root of tx receipts.
func (h *Header) ReceiptsRoot() cry.Hash {
	return h.subject.ReceiptsRoot
}

// EncodeRLP implements rlp.Encoder
func (h *Header) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		h.subject,
		h.signature,
	})
}

// Hash computes hash of header (block hash).
func (h *Header) Hash() cry.Hash {
	if cached := h.cache.hash; cached != nil {
		return *cached
	}

	hw := cry.NewHasher()
	rlp.Encode(hw, h)

	var hash cry.Hash
	hw.Sum(hash[:0])

	if (cry.Hash{}) == h.subject.ParentHash {
		// genesis block
		(*blockHash)(&hash).setBlockNumber(0)
	} else {
		parentNum := blockHash(h.subject.ParentHash).blockNumber()
		(*blockHash)(&hash).setBlockNumber(parentNum + 1)
	}

	h.cache.hash = &hash
	return hash
}

// HashForSigning computes hash of all header fields excluding signature.
func (h *Header) HashForSigning() cry.Hash {
	hw := cry.NewHasher()
	rlp.Encode(hw, &h.subject)

	var hash cry.Hash
	hw.Sum(hash[:0])
	return hash
}

// WithSignature create a new Header object with signature set.
func (h *Header) WithSignature(sig []byte) *Header {
	return &Header{
		subject:   h.subject,
		signature: append([]byte(nil), sig...),
	}
}

// Signer returns signer of this block.
func (h *Header) Signer() (*acc.Address, error) {
	if len(h.signature) == 0 {
		return nil, errors.New("not signed")
	}
	if signer := h.cache.signer; signer != nil {
		cpy := *signer
		return &cpy, nil
	}
	signer, err := dsa.Signer(h.HashForSigning(), h.signature)
	if err != nil {
		return nil, err
	}
	h.cache.signer = signer
	cpy := *signer
	return &cpy, nil
}

// HeaderDecoder to decode header from bytes.
// Since Header is immutable, it's not suitable to implement rlp.Decoder.
type HeaderDecoder struct {
	// decoded header
	Result *Header
}

// DecodeRLP implements rlp.Decoder.
func (d *HeaderDecoder) DecodeRLP(s *rlp.Stream) error {
	payload := struct {
		Subject subject
		Sig     []byte
	}{}

	if err := s.Decode(&payload); err != nil {
		return err
	}
	d.Result = &Header{
		subject:   payload.Subject,
		signature: payload.Sig,
	}
	return nil
}

// blockHash which first 4 bytes are over written by block number (big endian).
type blockHash cry.Hash

func (bh *blockHash) setBlockNumber(n uint32) {
	binary.BigEndian.PutUint32(bh[:4], n)
}

func (bh blockHash) blockNumber() uint32 {
	return binary.BigEndian.Uint32(bh[:4])
}

package block

import (
	"encoding/binary"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/thor"
)

var _ cry.Signable = (*Header)(nil)

// Header contains almost all information about a block, except body.
// It's immutable.
type Header struct {
	content headerContent

	cache struct {
		hash *thor.Hash
	}
}

// headerContent content of header
type headerContent struct {
	ParentHash  thor.Hash
	Timestamp   uint64
	TotalScore  uint64
	GasLimit    uint64
	GasUsed     uint64
	Beneficiary thor.Address

	TxsRoot      thor.Hash
	StateRoot    thor.Hash
	ReceiptsRoot thor.Hash

	Signature []byte
}

// ParentHash returns hash of parent block.
func (h *Header) ParentHash() thor.Hash {
	return h.content.ParentHash
}

// Number returns sequential number of this block.
func (h *Header) Number() uint32 {
	if (thor.Hash{}) == h.content.ParentHash {
		// genesis block
		return 0
	}
	// inferred from parent hash
	return Number(h.content.ParentHash) + 1
}

// Timestamp returns timestamp of this block.
func (h *Header) Timestamp() uint64 {
	return h.content.Timestamp
}

// TotalScore returns total score that cumulated from genesis block to this one.
func (h *Header) TotalScore() uint64 {
	return h.content.TotalScore
}

// GasLimit returns gas limit of this block.
func (h *Header) GasLimit() uint64 {
	return h.content.GasLimit
}

// GasUsed returns gas used by txs.
func (h *Header) GasUsed() uint64 {
	return h.content.GasUsed
}

// Beneficiary returns reward recipient.
func (h *Header) Beneficiary() thor.Address {
	return h.content.Beneficiary
}

// TxsRoot returns merkle root of txs contained in this block.
func (h *Header) TxsRoot() thor.Hash {
	return h.content.TxsRoot
}

// StateRoot returns account state merkle root just afert this block being applied.
func (h *Header) StateRoot() thor.Hash {
	return h.content.StateRoot
}

// ReceiptsRoot returns merkle root of tx receipts.
func (h *Header) ReceiptsRoot() thor.Hash {
	return h.content.ReceiptsRoot
}

// Hash computes hash of header (block hash).
func (h *Header) Hash() thor.Hash {
	if cached := h.cache.hash; cached != nil {
		return *cached
	}

	hw := cry.NewHasher()
	rlp.Encode(hw, h)

	var hash thor.Hash
	hw.Sum(hash[:0])

	// overwrite first 4 bytes of block hash to block number.
	binary.BigEndian.PutUint32(hash[:4], h.Number())

	h.cache.hash = &hash
	return hash
}

// SigningHash computes hash of all header fields excluding signature.
func (h *Header) SigningHash() thor.Hash {
	hw := cry.NewHasher()
	rlp.Encode(hw, []interface{}{
		h.content.ParentHash,
		h.content.Timestamp,
		h.content.TotalScore,
		h.content.GasLimit,
		h.content.GasUsed,
		h.content.Beneficiary,

		h.content.TxsRoot,
		h.content.StateRoot,
		h.content.ReceiptsRoot,
	})

	var hash thor.Hash
	hw.Sum(hash[:0])
	return hash
}

// Signature returns signature.
func (h *Header) Signature() []byte {
	return append([]byte(nil), h.content.Signature...)
}

// WithSignature create a new Header object with signature set.
func (h *Header) WithSignature(sig []byte) *Header {
	content := h.content
	content.Signature = append([]byte(nil), sig...)
	return &Header{
		content: content,
	}
}

// EncodeRLP implements rlp.Encoder
func (h *Header) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &h.content)
}

// DecodeRLP implements rlp.Decoder.
func (h *Header) DecodeRLP(s *rlp.Stream) error {
	var content headerContent

	if err := s.Decode(&content); err != nil {
		return err
	}
	*h = Header{
		content: content,
	}
	return nil
}

// Number extract block number from block hash.
func Number(hash thor.Hash) uint32 {
	// first 4 bytes are over written by block number (big endian).
	return binary.BigEndian.Uint32(hash[:4])
}

package block

import (
	"encoding/binary"
	"io"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/thor"
)

const signerCacheSize = 1024

var signerCache = cache.NewLRU(signerCacheSize)

// Header contains almost all information about a block, except block body.
// It's immutable.
type Header struct {
	body headerBody

	cache struct {
		signingHash *thor.Hash
		signer      *thor.Address
		id          *thor.Hash
	}
}

// headerBody body of header
type headerBody struct {
	ParentID    thor.Hash
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

// ParentID returns id of parent block.
func (h *Header) ParentID() thor.Hash {
	return h.body.ParentID
}

// Number returns sequential number of this block.
func (h *Header) Number() uint32 {
	if (thor.Hash{}) == h.body.ParentID {
		// genesis block
		return 0
	}
	// inferred from parent id
	return Number(h.body.ParentID) + 1
}

// Timestamp returns timestamp of this block.
func (h *Header) Timestamp() uint64 {
	return h.body.Timestamp
}

// TotalScore returns total score that cumulated from genesis block to this one.
func (h *Header) TotalScore() uint64 {
	return h.body.TotalScore
}

// GasLimit returns gas limit of this block.
func (h *Header) GasLimit() uint64 {
	return h.body.GasLimit
}

// GasUsed returns gas used by txs.
func (h *Header) GasUsed() uint64 {
	return h.body.GasUsed
}

// Beneficiary returns reward recipient.
func (h *Header) Beneficiary() thor.Address {
	return h.body.Beneficiary
}

// TxsRoot returns merkle root of txs contained in this block.
func (h *Header) TxsRoot() thor.Hash {
	return h.body.TxsRoot
}

// StateRoot returns account state merkle root just afert this block being applied.
func (h *Header) StateRoot() thor.Hash {
	return h.body.StateRoot
}

// ReceiptsRoot returns merkle root of tx receipts.
func (h *Header) ReceiptsRoot() thor.Hash {
	return h.body.ReceiptsRoot
}

// ID computes id of block.
// The block ID is defined as: blockNumber + hash(signingHash, signer)[4:].
func (h *Header) ID() (id thor.Hash) {
	if cached := h.cache.id; cached != nil {
		return *cached
	}
	defer func() {
		// overwrite first 4 bytes of block hash to block number.
		binary.BigEndian.PutUint32(id[:], h.Number())
		h.cache.id = &id
	}()

	if h.Number() == 0 {
		// genesis
		id = h.SigningHash()
		return
	}

	signer, err := h.Signer()
	if err != nil {
		return
	}

	hw := sha3.NewKeccak256()
	hw.Write(h.SigningHash().Bytes())
	hw.Write(signer.Bytes())
	hw.Sum(id[:0])

	return
}

// SigningHash computes hash of all header fields excluding signature.
func (h *Header) SigningHash() (hash thor.Hash) {
	if cached := h.cache.signingHash; cached != nil {
		return *cached
	}
	defer func() { h.cache.signingHash = &hash }()

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, []interface{}{
		h.body.ParentID,
		h.body.Timestamp,
		h.body.TotalScore,
		h.body.GasLimit,
		h.body.GasUsed,
		h.body.Beneficiary,

		h.body.TxsRoot,
		h.body.StateRoot,
		h.body.ReceiptsRoot,
	})
	hw.Sum(hash[:0])
	return
}

// Signature returns signature.
func (h *Header) Signature() []byte {
	return append([]byte(nil), h.body.Signature...)
}

// WithSignature create a new Header object with signature set.
func (h *Header) WithSignature(sig []byte) *Header {
	cpy := Header{body: h.body}
	cpy.body.Signature = append([]byte(nil), sig...)
	return &cpy
}

// Signer extract signer of the block from signature.
func (h *Header) Signer() (signer thor.Address, err error) {
	if cached := h.cache.signer; cached != nil {
		return *cached, nil
	}
	defer func() {
		if err == nil {
			h.cache.signer = &signer
		}
	}()

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, h)
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
	pub, err := crypto.SigToPub(h.SigningHash().Bytes(), h.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// EncodeRLP implements rlp.Encoder
func (h *Header) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &h.body)
}

// DecodeRLP implements rlp.Decoder.
func (h *Header) DecodeRLP(s *rlp.Stream) error {
	var body headerBody

	if err := s.Decode(&body); err != nil {
		return err
	}
	*h = Header{body: body}
	return nil
}

// Number extract block number from block id.
func Number(blockID thor.Hash) uint32 {
	// first 4 bytes are over written by block number (big endian).
	return binary.BigEndian.Uint32(blockID[:])
}

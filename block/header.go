// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vrf"
)

// Header contains almost all information about a block, except block body.
// It's immutable.
type Header struct {
	body headerBody

	cache struct {
		signingHash atomic.Value
		id          atomic.Value
		pubkey      atomic.Value
		beta        atomic.Value
	}
}

// headerBody body of header
type headerBody struct {
	ParentID    thor.Bytes32
	Timestamp   uint64
	GasLimit    uint64
	Beneficiary thor.Address

	GasUsed    uint64
	TotalScore uint64

	TxsRootFeatures txsRootFeatures
	StateRoot       thor.Bytes32
	ReceiptsRoot    thor.Bytes32

	Signature []byte

	Extension extension
}

// ParentID returns id of parent block.
func (h *Header) ParentID() thor.Bytes32 {
	return h.body.ParentID
}

// Number returns sequential number of this block.
func (h *Header) Number() uint32 {
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
func (h *Header) TxsRoot() thor.Bytes32 {
	return h.body.TxsRootFeatures.Root
}

// TxsFeatures returns supported txs features.
func (h *Header) TxsFeatures() tx.Features {
	return h.body.TxsRootFeatures.Features
}

// StateRoot returns account state merkle root just after this block being applied.
func (h *Header) StateRoot() thor.Bytes32 {
	return h.body.StateRoot
}

// ReceiptsRoot returns merkle root of tx receipts.
func (h *Header) ReceiptsRoot() thor.Bytes32 {
	return h.body.ReceiptsRoot
}

// ID computes id of block.
// The block ID is defined as: blockNumber + hash(signingHash, signer)[4:].
func (h *Header) ID() (id thor.Bytes32) {
	if cached := h.cache.id.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() {
		// overwrite first 4 bytes of block hash to block number.
		binary.BigEndian.PutUint32(id[:], h.Number())
		h.cache.id.Store(id)
	}()

	signer, err := h.Signer()
	if err != nil {
		return
	}

	return thor.Blake2b(h.SigningHash().Bytes(), signer[:])
}

// SigningHash computes hash of all header fields excluding signature.
func (h *Header) SigningHash() (hash thor.Bytes32) {
	if cached := h.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { h.cache.signingHash.Store(hash) }()

	return thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, []interface{}{
			&h.body.ParentID,
			h.body.Timestamp,
			h.body.GasLimit,
			&h.body.Beneficiary,

			h.body.GasUsed,
			h.body.TotalScore,

			&h.body.TxsRootFeatures,
			&h.body.StateRoot,
			&h.body.ReceiptsRoot,
		})
	})
}

// Signature returns signature.
func (h *Header) Signature() []byte {
	return append([]byte(nil), h.body.Signature...)
}

// withSignature create a new Header object with signature set.
func (h *Header) withSignature(sig []byte) *Header {
	cpy := Header{body: h.body}
	cpy.body.Signature = append([]byte(nil), sig...)
	return &cpy
}

// pubkey recover leader's public key.
func (h *Header) pubkey() (pubkey *ecdsa.PublicKey, err error) {
	if cached := h.cache.pubkey.Load(); cached != nil {
		return cached.(*ecdsa.PublicKey), nil
	}

	defer func() {
		if err == nil {
			h.cache.pubkey.Store(pubkey)
		}
	}()

	if len(h.body.Signature) < 65 {
		return nil, errors.New("invalid signature length")
	}

	return crypto.SigToPub(h.SigningHash().Bytes(), ComplexSignature(h.body.Signature).Signature())
}

// Signer extract signer of the block from signature.
func (h *Header) Signer() (thor.Address, error) {
	if h.Number() == 0 {
		// special case for genesis block
		return thor.Address{}, nil
	}

	pub, err := h.pubkey()
	if err != nil {
		return thor.Address{}, err
	}

	return thor.Address(crypto.PubkeyToAddress(*pub)), nil
}

// Alpha returns the alpha in the header.
func (h *Header) Alpha() []byte {
	return h.body.Extension.Alpha
}

// COM returns whether the packer votes COM.
func (h *Header) COM() bool {
	return h.body.Extension.COM
}

// Beta verifies the VRF proof in header's signature and returns the beta.
func (h *Header) Beta() (beta []byte, err error) {
	if h.Number() == 0 || len(h.body.Signature) == 65 {
		return
	}

	if cached := h.cache.beta.Load(); cached != nil {
		return cached.([]byte), nil
	}
	defer func() {
		if err == nil {
			h.cache.beta.Store(beta)
		}
	}()

	if len(h.body.Signature) != ComplexSigSize {
		return nil, errors.New("invalid signature length")
	}
	pub, err := h.pubkey()
	if err != nil {
		return
	}

	return vrf.Verify(pub, h.body.Extension.Alpha, ComplexSignature(h.body.Signature).Proof())
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

func (h *Header) String() string {
	var signerStr string
	if signer, err := h.Signer(); err != nil {
		signerStr = "N/A"
	} else {
		signerStr = signer.String()
	}

	return fmt.Sprintf(`Header(%v):
	Number:         %v
	ParentID:       %v
	Timestamp:      %v
	Signer:         %v
	Beneficiary:    %v
	GasLimit:       %v
	GasUsed:        %v
	TotalScore:     %v
	TxsRoot:        %v
	TxsFeatures:    %v
	StateRoot:      %v
	ReceiptsRoot:   %v
	Alpha:          0x%x
	COM:            %v
	Signature:      0x%x`, h.ID(), h.Number(), h.body.ParentID, h.body.Timestamp, signerStr,
		h.body.Beneficiary, h.body.GasLimit, h.body.GasUsed, h.body.TotalScore,
		h.body.TxsRootFeatures.Root, h.body.TxsRootFeatures.Features, h.body.StateRoot, h.body.ReceiptsRoot, h.body.Extension.Alpha, h.body.Extension.COM, h.body.Signature)
}

// BetterThan return if this block is better than other one.
func (h *Header) BetterThan(other *Header) bool {
	s1 := h.TotalScore()
	s2 := other.TotalScore()

	if s1 > s2 {
		return true
	}
	if s1 < s2 {
		return false
	}
	// total scores are equal

	// smaller ID is preferred, since block with smaller ID usually has larger average score.
	// also, it's a deterministic decision.
	return bytes.Compare(h.ID().Bytes(), other.ID().Bytes()) < 0
}

// Number extract block number from block id.
func Number(blockID thor.Bytes32) uint32 {
	// first 4 bytes are over written by block number (big endian).
	return binary.BigEndian.Uint32(blockID[:])
}

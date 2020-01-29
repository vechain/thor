// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
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
		signer      atomic.Value
		id          atomic.Value
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

	// Committee         []uint8
	VrfProofs         []*vrf.Proof
	SigOnBlockSummary []byte
	SigsOnEndorsement [][]byte

	Signature []byte
}

// Seed returns a random seed for computing the beacon
func (h *Header) Seed() thor.Bytes32 {
	return h.ID()
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

// StateRoot returns account state merkle root just afert this block being applied.
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

	hw := thor.NewBlake2b()
	hw.Write(h.SigningHash().Bytes())
	hw.Write(signer.Bytes())
	hw.Sum(id[:0])

	return
}

// SigningHash computes hash of all header fields excluding signature.
func (h *Header) SigningHash() (hash thor.Bytes32) {
	if cached := h.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { h.cache.signingHash.Store(hash) }()

	sort.Sort(vrf.Proofs(h.body.VrfProofs))

	hw := thor.NewBlake2b()
	rlp.Encode(hw, []interface{}{
		h.body.ParentID,
		h.body.Timestamp,
		h.body.GasLimit,
		h.body.Beneficiary,

		h.body.GasUsed,
		h.body.TotalScore,

		&h.body.TxsRootFeatures,
		h.body.StateRoot,
		h.body.ReceiptsRoot,

		// h.body.Committee,
		h.body.VrfProofs,
		h.body.SigOnBlockSummary,
		h.body.SigsOnEndorsement,
	})
	hw.Sum(hash[:0])
	return
}

// Signature returns signature.
func (h *Header) Signature() []byte {
	return append([]byte(nil), h.body.Signature...)
}

// SigOnBlockSummary returns signature on block summary
func (h *Header) SigOnBlockSummary() []byte {
	return append([]byte(nil), h.body.SigOnBlockSummary...)
}

// VrfProofs returns vrf proofs
func (h *Header) VrfProofs() []*vrf.Proof {
	return h.body.VrfProofs
}

func (h *Header) SigsOnEndoresment() [][]byte {
	return h.body.SigsOnEndorsement
}

// WithSignature create a new Header object with signature set.
func (h *Header) WithSignature(sig []byte) *Header {
	cpy := Header{body: h.body}
	cpy.body.Signature = append([]byte(nil), sig...)
	return &cpy
}

// Signer extract signer of the block from signature.
func (h *Header) Signer() (signer thor.Address, err error) {
	if h.Number() == 0 {
		// special case for genesis block
		return thor.Address{}, nil
	}

	if cached := h.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() {
		if err == nil {
			h.cache.signer.Store(signer)
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

func (h *Header) String() string {
	var signerStr string
	if signer, err := h.Signer(); err != nil {
		signerStr = "N/A"
	} else {
		signerStr = signer.String()
	}

	s := fmt.Sprintf(`Header(%v):
	Number:         	%v
	ParentID:       	%v
	Timestamp:      	%v
	Signer:         	%v
	Beneficiary:    	%v
	GasLimit:       	%v
	GasUsed:        	%v
	TotalScore:     	%v
	TxsRoot:        	%v
	TxsFeatures:    	%v
	StateRoot:      	%v
	ReceiptsRoot:   	%v
	Signature:      	0x%x
	`, h.ID(), h.Number(), h.body.ParentID, h.body.Timestamp, signerStr,
		h.body.Beneficiary, h.body.GasLimit, h.body.GasUsed, h.body.TotalScore,
		h.body.TxsRootFeatures.Root, h.body.TxsRootFeatures.Features, h.body.StateRoot, h.body.ReceiptsRoot,
		h.body.Signature,
	)

	for i := range h.body.VrfProofs {
		s = s + fmt.Sprintf(`
	{
		VRF Proof:          0x%x
		SigOnBlockSummary:  0x%x
		SigOnEndorsement:   0x%x
	}`, h.body.VrfProofs[i].Bytes(), h.body.SigOnBlockSummary, h.body.SigsOnEndorsement[i])
	}

	return s
}

// Number extract block number from block id.
func Number(blockID thor.Bytes32) uint32 {
	// first 4 bytes are over written by block number (big endian).
	return binary.BigEndian.Uint32(blockID[:])
}

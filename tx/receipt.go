// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

var (
	errEmptyTypedReceipt = errors.New("empty typed receipt bytes")
	errShortTypedReceipt = errors.New("typed receipt too short")
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// transaction type this receipt is associated with
	Type byte
	ReceiptBody
}

type ReceiptBody struct {
	// gas used by this tx
	GasUsed uint64
	// the one who paid for gas
	GasPayer thor.Address
	// energy paid for used gas
	Paid *big.Int
	// energy reward given to block proposer
	Reward *big.Int
	// if the tx reverted
	Reverted bool
	// outputs of clauses in tx
	Outputs []*Output
}

// Output output of clause execution.
type Output struct {
	// events produced by the clause
	Events Events
	// transfer occurred in clause
	Transfers Transfers
}

// Receipts slice of receipts.
type Receipts []*Receipt

// RootHash computes merkle root hash of receipts.
func (rs Receipts) RootHash() thor.Bytes32 {
	if len(rs) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableReceipts(rs))
}

// implements DerivableList
type derivableReceipts Receipts

func (rs derivableReceipts) Len() int {
	return len(rs)
}
func (rs derivableReceipts) GetRlp(i int) []byte {
	data, err := rs[i].MarshalBinary()
	if err != nil {
		panic(err)
	}
	return data
}

// EncodeRLP implements rlp.Encoder, and flattens the consensus fields of a receipt
// into an RLP stream. If no post state is present, byzantium fork is assumed.
func (r *Receipt) EncodeRLP(w io.Writer) error {
	data := &ReceiptBody{
		r.GasUsed, r.GasPayer, r.Paid, r.Reward, r.Reverted, r.Outputs,
	}
	if r.Type == TypeLegacy {
		return rlp.Encode(w, data)
	}

	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	buf.WriteByte(r.Type)
	if err := rlp.Encode(buf, data); err != nil {
		return err
	}
	return rlp.Encode(w, buf.Bytes())
}

// DecodeRLP implements rlp.Decoder, and loads the consensus fields of a receipt
// from an RLP stream.
func (r *Receipt) DecodeRLP(s *rlp.Stream) error {
	kind, _, err := s.Kind()
	switch {
	case err != nil:
		return err
	case kind == rlp.List:
		// It's a legacy receipt.
		var dec ReceiptBody
		if err := s.Decode(&dec); err != nil {
			return err
		}
		r.Type = TypeLegacy
		r.setFromRLP(dec)
	case kind == rlp.String:
		// It's an EIP-2718 typed tx receipt.
		b, err := s.Bytes()
		if err != nil {
			return err
		}
		if len(b) == 0 {
			return errEmptyTypedReceipt
		}
		r.Type = b[0]
		switch r.Type {
		case TypeDynamicFee:
			var dec ReceiptBody
			if err := rlp.DecodeBytes(b[1:], &dec); err != nil {
				return err
			}
			r.setFromRLP(dec)
		default:
			return ErrTxTypeNotSupported
		}
	default:
		return rlp.ErrExpectedList
	}

	return nil
}

func (r *Receipt) setFromRLP(dec ReceiptBody) {
	r.GasUsed = dec.GasUsed
	r.GasPayer = dec.GasPayer
	r.Paid = dec.Paid
	r.Reward = dec.Reward
	r.Reverted = dec.Reverted
	r.Outputs = dec.Outputs
}

// MarshalBinary returns the consensus encoding of the receipt.
func (r *Receipt) MarshalBinary() ([]byte, error) {
	if r.Type == TypeLegacy {
		return rlp.EncodeToBytes(r)
	}
	data := &ReceiptBody{
		r.GasUsed, r.GasPayer, r.Paid, r.Reward, r.Reverted, r.Outputs,
	}
	var buf bytes.Buffer
	err := r.encodeTyped(data, &buf)
	return buf.Bytes(), err
}

// UnmarshalBinary decodes the consensus encoding of receipts.
// It supports legacy RLP receipts and EIP-2718 typed receipts.
func (r *Receipt) UnmarshalBinary(b []byte) error {
	if len(b) > 0 && b[0] > 0x7f {
		// It's a legacy receipt decode the RLP
		var data ReceiptBody
		err := rlp.DecodeBytes(b, &data)
		if err != nil {
			return err
		}
		r.Type = TypeLegacy
		r.setFromRLP(data)
		return nil
	}
	// It's an EIP2718 typed transaction envelope.
	return r.decodeTyped(b)
}

// encodeTyped writes the canonical encoding of a typed receipt to w.
func (r *Receipt) encodeTyped(data *ReceiptBody, w *bytes.Buffer) error {
	w.WriteByte(r.Type)
	return rlp.Encode(w, data)
}

// decodeTyped decodes a typed receipt from the canonical format.
func (r *Receipt) decodeTyped(b []byte) error {
	if len(b) <= 1 {
		return errShortTypedReceipt
	}
	switch b[0] {
	case TypeDynamicFee:
		var data ReceiptBody
		err := rlp.DecodeBytes(b[1:], &data)
		if err != nil {
			return err
		}
		r.Type = b[0]
		r.setFromRLP(data)
		return nil
	default:
		return ErrTxTypeNotSupported
	}
}

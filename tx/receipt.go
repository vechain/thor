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

var errShortTypedReceipt = errors.New("typed receipt too short")

// Receipt represents the results of a transaction.
type Receipt struct {
	// transaction type this receipt is associated with
	Type byte
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

// receiptRLP helper struct for RLP encoding.
type receiptRLP struct {
	GasUsed  uint64
	GasPayer thor.Address
	Paid     *big.Int
	Reward   *big.Int
	Reverted bool
	Outputs  []*Output
}

// Output output of clause execution.
type Output struct {
	// events produced by the clause
	Events Events
	// transfer occurred in clause
	Transfers Transfers
}

// MarshalBinary returns the consensus encoding of the receipt.
func (r *Receipt) MarshalBinary() ([]byte, error) {
	data := receiptRLP{r.GasUsed, r.GasPayer, r.Paid, r.Reward, r.Reverted, r.Outputs}
	if r.Type == TypeLegacy {
		return rlp.EncodeToBytes(&data)
	}

	var buf bytes.Buffer
	err := r.encodeTyped(&data, &buf)
	return buf.Bytes(), err
}

// encodeTyped writes the canonical encoding of a typed receipt to w.
func (r *Receipt) encodeTyped(data *receiptRLP, w *bytes.Buffer) error {
	w.WriteByte(r.Type)
	return rlp.Encode(w, data)
}

// EncodeRLP implements rlp.Encoder, and flattens the consensus fields of a receipt
// into an RLP stream. If no post state is present, byzantium fork is assumed.
func (r *Receipt) EncodeRLP(w io.Writer) error {
	data := receiptRLP{r.GasUsed, r.GasPayer, r.Paid, r.Reward, r.Reverted, r.Outputs}

	if r.Type == TypeLegacy {
		return rlp.Encode(w, &data)
	}
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()

	if err := r.encodeTyped(&data, buf); err != nil {
		return err
	}

	return rlp.Encode(w, buf.Bytes())
}

// UnmarshalBinary decodes the consensus encoding of receipts.
// It supports legacy RLP receipts and EIP-2718 typed receipts.
func (r *Receipt) UnmarshalBinary(b []byte) error {
	if len(b) > 0 && b[0] > 0x7f {
		// It's a legacy receipt decode the RLP
		var data receiptRLP
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

// decodeTyped decodes a typed receipt from the canonical format.
func (r *Receipt) decodeTyped(b []byte) error {
	if len(b) <= 1 {
		return errShortTypedReceipt
	}
	switch b[0] {
	case TypeDynamicFee:
		var data receiptRLP
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

func (r *Receipt) setFromRLP(data receiptRLP) {
	r.GasUsed, r.GasPayer, r.Paid, r.Reward, r.Reverted, r.Outputs = data.GasUsed, data.GasPayer, data.Paid, data.Reward, data.Reverted, data.Outputs
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
		var dec receiptRLP
		if err := s.Decode(&dec); err != nil {
			return err
		}
		r.Type = TypeLegacy
		r.setFromRLP(dec)
	case kind == rlp.Byte:
		return errShortTypedReceipt
	default:
		// It's an EIP-2718 typed tx receipt.
		b, err := s.Bytes()
		if err != nil {
			return err
		}
		if len(b) < 1 {
			return errShortTypedReceipt
		}
		r.Type = b[0]
		switch r.Type {
		case TypeDynamicFee:
			var dec receiptRLP
			if err := rlp.DecodeBytes(b[1:], &dec); err != nil {
				return err
			}
			r.setFromRLP(dec)
		default:
			return ErrTxTypeNotSupported
		}
	}
	return nil
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

func (rs derivableReceipts) EncodeIndex(i int) []byte {
	data, err := rs[i].MarshalBinary()
	if err != nil {
		panic(err)
	}
	return data
}

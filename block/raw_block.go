// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/tx"
)

// RawBlock holds a decoded header and raw (undecoded) transaction bytes.
// It implements a two-phase decode pattern: Phase 1 (DecodeRLP) validates the
// block's RLP structure and decodes only the header; Phase 2 (Decode) performs
// the expensive transaction deserialization.
//
// This allows callers to inspect the header (e.g. verify block ID, check sequence
// number) before committing to the costly transaction decode, and to reject
// malformed or unexpected blocks cheaply.
type RawBlock struct {
	header  *Header
	rawTxs  rlp.RawValue
	txCount int
	size    uint64
}

// DecodeRLP implements rlp.Decoder. It performs Phase 1 of the two-phase decode:
// validates the block RLP structure (outer list with exactly 2 items), decodes the
// header, and counts transactions via rlp.CountValues without decoding them.
func (rb *RawBlock) DecodeRLP(s *rlp.Stream) error {
	contentSize, err := s.List()
	if err != nil {
		return err
	}

	var header Header
	if err := s.Decode(&header); err != nil {
		return err
	}

	rawTxs, err := s.Raw()
	if err != nil {
		return err
	}

	if err := s.ListEnd(); err != nil {
		return err
	}

	txContent, _, err := rlp.SplitList(rawTxs)
	if err != nil {
		return err
	}

	var txCount int
	if len(txContent) > 0 {
		txCount, err = rlp.CountValues(txContent)
		if err != nil {
			return err
		}
	}

	*rb = RawBlock{
		header:  &header,
		rawTxs:  rawTxs,
		txCount: txCount,
		size:    rlp.ListSize(contentSize),
	}
	return nil
}

// Header returns the already-decoded block header.
func (rb *RawBlock) Header() *Header {
	return rb.header
}

// Decode performs Phase 2: decodes the raw transaction bytes into a fully
// materialized Block. This is the expensive step that should only be called
// after Phase 1 checks (ID verification, sequence checks, etc.) have passed.
func (rb *RawBlock) Decode() (*Block, error) {
	var txs tx.Transactions
	if err := rlp.DecodeBytes(rb.rawTxs, &txs); err != nil {
		return nil, err
	}

	b := &Block{
		header: rb.header,
		txs:    txs,
	}
	b.cache.size.Store(rb.size)
	return b, nil
}

// DecodeRawBlock decodes raw RLP bytes into a RawBlock using the two-phase
// approach. Only the header is decoded; transactions remain as raw bytes until
// Decode() is called.
func DecodeRawBlock(data []byte) (*RawBlock, error) {
	var rb RawBlock
	if err := rlp.DecodeBytes(data, &rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

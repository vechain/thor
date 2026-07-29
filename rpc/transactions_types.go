// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/thor"
)

// TxHashParams holds a single transaction hash parameter.
type TxHashParams struct {
	Hash thor.Bytes32
}

func (p *TxHashParams) UnmarshalJSON(data []byte) error {
	var raw [1]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("expected [txHash]")
	}
	var hashStr string
	if err := json.Unmarshal(raw[0], &hashStr); err != nil {
		return fmt.Errorf("invalid tx hash")
	}
	hash, err := thor.ParseBytes32(hashStr)
	if err != nil {
		return fmt.Errorf("invalid tx hash: %w", err)
	}
	p.Hash = hash
	return nil
}

// BlockTagAndIndexParams holds a block identifier and a hex-encoded transaction index,
// used by eth_getTransactionByBlockHashAndIndex and eth_getTransactionByBlockNumberAndIndex.
type BlockTagAndIndexParams struct {
	Tag   string
	Index uint64
}

func (p *BlockTagAndIndexParams) UnmarshalJSON(data []byte) error {
	var raw [2]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("expected [blockTag, index]")
	}
	if err := json.Unmarshal(raw[0], &p.Tag); err != nil {
		return fmt.Errorf("invalid block tag")
	}
	var idxStr string
	if err := json.Unmarshal(raw[1], &idxStr); err != nil {
		return fmt.Errorf("invalid index")
	}
	idx, err := hexutil.DecodeUint64(idxStr)
	if err != nil {
		return fmt.Errorf("invalid index: %w", err)
	}
	p.Index = idx
	return nil
}

// RawTxParams holds the hex-decoded bytes of a raw signed transaction,
// used by eth_sendRawTransaction.
type RawTxParams struct {
	Raw []byte
}

func (p *RawTxParams) UnmarshalJSON(data []byte) error {
	var raw [1]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("expected [rawTx]")
	}
	var hexStr string
	if err := json.Unmarshal(raw[0], &hexStr); err != nil {
		return fmt.Errorf("invalid raw transaction")
	}
	decoded, err := hexutil.Decode(hexStr)
	if err != nil {
		return fmt.Errorf("invalid hex encoding: %w", err)
	}
	p.Raw = decoded
	return nil
}

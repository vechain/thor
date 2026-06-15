// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"fmt"
)

// BlockQueryParams holds the parameters for eth_getBlockByHash and eth_getBlockByNumber.
type BlockQueryParams struct {
	Tag     string
	FullTxs bool
}

func (p *BlockQueryParams) UnmarshalJSON(data []byte) error {
	var raw [2]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("expected [blockTag, fullTransactions]")
	}
	if err := json.Unmarshal(raw[0], &p.Tag); err != nil {
		return fmt.Errorf("invalid block tag")
	}
	if err := json.Unmarshal(raw[1], &p.FullTxs); err != nil {
		return fmt.Errorf("invalid fullTransactions flag")
	}
	return nil
}

// BlockTagParams holds a single block tag parameter, used by methods that accept
// only a block identifier (hash, number, or tag such as "latest").
type BlockTagParams struct {
	Tag string
}

func (p *BlockTagParams) UnmarshalJSON(data []byte) error {
	var raw [1]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("expected [blockTag]")
	}
	if err := json.Unmarshal(raw[0], &p.Tag); err != nil {
		return fmt.Errorf("invalid block tag")
	}
	return nil
}

// BlockReceiptsParams holds the single BlockNumberOrHash argument accepted by
// eth_getBlockReceipts. The block parameter accepts the same forms as
// Ethereum's BlockNumberOrHash (string tag, hex number, hash, or object).
type BlockReceiptsParams struct {
	Block BlockNumberOrHash
}

func (p *BlockReceiptsParams) UnmarshalJSON(data []byte) error {
	var raw [1]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("expected [blockNrOrHash]")
	}
	if err := json.Unmarshal(raw[0], &p.Block); err != nil {
		return fmt.Errorf("invalid block parameter: %w", err)
	}
	return nil
}

// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"fmt"

	"github.com/vechain/thor/v2/thor"
)

// AddressAndTagParams holds an account address and an optional block reference,
// used by eth_getBalance, eth_getCode, and eth_getTransactionCount.
// Block accepts the same forms as Ethereum's BlockNumberOrHash and defaults to
// "latest" when omitted or null.
type AddressAndTagParams struct {
	Address thor.Address
	Block   BlockNumberOrHash
}

func (p *AddressAndTagParams) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil || len(raws) < 1 {
		return fmt.Errorf("expected [address, blockNrOrHash?]")
	}
	var addrStr string
	if err := json.Unmarshal(raws[0], &addrStr); err != nil {
		return fmt.Errorf("invalid address")
	}
	addr, err := thor.ParseAddress(addrStr)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	p.Address = addr
	p.Block = LatestBlockNumberOrHash()
	if len(raws) >= 2 && string(raws[1]) != "null" {
		if err := json.Unmarshal(raws[1], &p.Block); err != nil {
			return fmt.Errorf("invalid block parameter: %w", err)
		}
	}
	return nil
}

// StorageAtParams holds an account address, a storage slot, and an optional block
// reference, used by eth_getStorageAt. Block defaults to "latest" when omitted
// or null and accepts the same forms as Ethereum's BlockNumberOrHash.
type StorageAtParams struct {
	Address thor.Address
	Slot    thor.Bytes32
	Block   BlockNumberOrHash
}

func (p *StorageAtParams) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil || len(raws) < 2 {
		return fmt.Errorf("expected [address, slot, blockNrOrHash?]")
	}
	var addrStr string
	if err := json.Unmarshal(raws[0], &addrStr); err != nil {
		return fmt.Errorf("invalid address")
	}
	addr, err := thor.ParseAddress(addrStr)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	p.Address = addr
	var slotStr string
	if err := json.Unmarshal(raws[1], &slotStr); err != nil {
		return fmt.Errorf("invalid slot")
	}
	slot, err := ParseBytes32Compact(slotStr)
	if err != nil {
		return fmt.Errorf("invalid slot: %w", err)
	}
	p.Slot = slot
	p.Block = LatestBlockNumberOrHash()
	if len(raws) >= 3 && string(raws[2]) != "null" {
		if err := json.Unmarshal(raws[2], &p.Block); err != nil {
			return fmt.Errorf("invalid block parameter: %w", err)
		}
	}
	return nil
}

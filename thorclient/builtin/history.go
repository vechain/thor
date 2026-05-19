// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

// History wraps the EIP-2935 historical-block-hash facade. The contract has
// no function selector — calldata is a single uint256 block number — so it
// is invoked via raw clauses rather than the ABI-based bind.Contract.
type History struct {
	client   *thorclient.Client
	address  thor.Address
	revision string
}

// NewHistory creates a client for the History (EIP-2935) builtin contract.
func NewHistory(client *thorclient.Client) *History {
	return &History{
		client:  client,
		address: builtin.History.Address,
	}
}

// Revision creates a new History instance bound to the specified revision.
func (h *History) Revision(rev string) *History {
	return &History{
		client:   h.client,
		address:  h.address,
		revision: rev,
	}
}

// Address returns the History contract address.
func (h *History) Address() thor.Address {
	return h.address
}

// BlockID returns the historical block ID for the given block number.
//
// Per EIP-2935 the call reverts (with empty return data) when num is in the
// future or older than SERVE_WINDOW (8191) blocks behind the best block.
func (h *History) BlockID(blockNumber uint32) (thor.Bytes32, error) {
	var data [32]byte
	new(big.Int).SetUint64(uint64(blockNumber)).FillBytes(data[:])
	return h.callBytes32(data[:])
}

// CallRaw invokes the History contract with arbitrary raw calldata and
// returns the raw response bytes. Useful for exercising invalid-length
// inputs that would not round-trip through the BlockID helper.
func (h *History) CallRaw(data []byte) ([]byte, error) {
	res, err := h.call(data)
	if err != nil {
		return nil, err
	}
	return hexutil.Decode(res.Data)
}

func (h *History) callBytes32(data []byte) (thor.Bytes32, error) {
	out, err := h.CallRaw(data)
	if err != nil {
		return thor.Bytes32{}, err
	}
	return thor.BytesToBytes32(out), nil
}

func (h *History) call(data []byte) (*api.CallResult, error) {
	body := &api.BatchCallData{
		Clauses: api.Clauses{
			{
				To:    &h.address,
				Data:  hexutil.Encode(data),
				Value: (*math.HexOrDecimal256)(big.NewInt(0)),
			},
		},
	}

	var opts []thorclient.Option
	if h.revision != "" {
		opts = append(opts, thorclient.Revision(h.revision))
	}

	results, err := h.client.InspectClauses(body, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect history clause: %w", err)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Reverted {
		if result.VMError != "" {
			return result, errors.New(result.VMError)
		}
		return result, errors.New("execution reverted")
	}
	if result.VMError != "" {
		return nil, fmt.Errorf("VM error: %s", result.VMError)
	}
	return result, nil
}

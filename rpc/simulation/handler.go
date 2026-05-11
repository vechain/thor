// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package simulation

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

// Handler implements eth_call and eth_estimateGas JSON-RPC methods.
type Handler struct {
	repo         *chain.Repository
	stater       *state.Stater
	forkConfig   *thor.ForkConfig
	callGasLimit uint64
}

// New creates a simulation Handler.
func New(repo *chain.Repository, stater *state.Stater, forkConfig *thor.ForkConfig, callGasLimit uint64) *Handler {
	return &Handler{repo: repo, stater: stater, forkConfig: forkConfig, callGasLimit: callGasLimit}
}

// Mount registers all simulation methods on the dispatcher.
func (h *Handler) Mount(s *rpc.Server) {
	s.Register("eth_call", h.ethCall)
	s.Register("eth_estimateGas", h.ethEstimateGas)
}

// CallArgs mirrors the Ethereum eth_call / eth_estimateGas parameter object.
type CallArgs struct {
	From     *common.Address `json:"from"`
	To       *common.Address `json:"to"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Data     hexutil.Bytes   `json:"data"`
}

func (h *Handler) ethCall(req rpc.Request) rpc.Response {
	args, tag, err := parseCallArgs(req.Params)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, err.Error())
	}

	out, _, execErr := h.simulate(args, tag, h.callGasLimit)
	if execErr != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, execErr.Error())
	}
	if out.VMErr != nil {
		return rpc.ErrResponseWithData(req.ID, rpc.CodeServerError, "execution reverted", hexutil.Encode(out.Data))
	}
	return rpc.OkResponse(req.ID, hexutil.Bytes(out.Data))
}

func (h *Handler) ethEstimateGas(req rpc.Request) rpc.Response {
	args, tag, err := parseCallArgs(req.Params)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, err.Error())
	}

	limit := h.callGasLimit
	if args.Gas != nil && uint64(*args.Gas) < limit {
		limit = uint64(*args.Gas)
	}

	// Run with full gas limit to determine if the call succeeds at all.
	out, _, execErr := h.simulate(args, tag, limit)
	if execErr != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, execErr.Error())
	}
	if out.VMErr != nil {
		return rpc.ErrResponseWithData(req.ID, rpc.CodeServerError, "execution reverted", hexutil.Encode(out.Data))
	}

	evmGasUsed := limit - out.LeftOverGas

	// PrepareClause does not charge intrinsic gas (tx base + per-clause overhead).
	// Add it explicitly so the estimate matches what the network will deduct.
	var to *thor.Address
	if args.To != nil {
		addr := thor.Address(*args.To)
		to = &addr
	}
	intrinsic, err := tx.IntrinsicGas(tx.NewClause(to).WithData(args.Data))
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	// Edge case: if the call uses exactly gasLimit (leftover == 0), this returns
	// callGasLimit + intrinsic — the absolute maximum. The estimate may still be too
	// low for the actual tx, but returning the ceiling is acceptable.
	return rpc.OkResponse(req.ID, hexutil.Uint64(evmGasUsed+intrinsic))
}

func (h *Handler) simulate(args CallArgs, tag string, gasLimit uint64) (*runtime.Output, *state.State, error) {
	summary, err := rpc.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return nil, nil, err
	}
	header := summary.Header

	st := h.stater.NewState(summary.Root())
	signer, _ := header.Signer()

	rt := runtime.New(
		h.repo.NewChain(header.ParentID()),
		st,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore(),
			BaseFee:     header.BaseFee(),
		},
		h.forkConfig,
	)

	var origin thor.Address
	if args.From != nil {
		origin = thor.Address(*args.From)
	}
	var gasPrice *big.Int
	if args.GasPrice != nil {
		gasPrice = (*big.Int)(args.GasPrice)
	} else {
		gasPrice = new(big.Int)
	}
	var value *big.Int
	if args.Value != nil {
		value = (*big.Int)(args.Value)
	} else {
		value = new(big.Int)
	}

	var to *thor.Address
	if args.To != nil {
		addr := thor.Address(*args.To)
		to = &addr
	}

	clause := tx.NewClause(to).WithData(args.Data).WithValue(value)
	txCtx := &xenv.TransactionContext{
		Origin:      origin,
		GasPrice:    gasPrice,
		ClauseCount: 1,
		Type:        tx.TypeEthDynamicFee,
	}

	exec, _ := rt.PrepareClause(clause, 0, gasLimit, txCtx)
	out, _, err := exec()
	return out, st, err
}

func parseCallArgs(raw json.RawMessage) (CallArgs, string, error) {
	var params []json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil || len(params) < 1 {
		return CallArgs{}, "", fmt.Errorf("expected [callArgs, blockTag?]")
	}
	var args CallArgs
	if err := json.Unmarshal(params[0], &args); err != nil {
		return CallArgs{}, "", fmt.Errorf("invalid call arguments: %w", err)
	}
	tag := "latest"
	if len(params) >= 2 {
		if err := json.Unmarshal(params[1], &tag); err != nil {
			return CallArgs{}, "", fmt.Errorf("invalid block tag")
		}
	}
	return args, tag, nil
}

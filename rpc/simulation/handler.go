// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package simulation

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
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
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_call", h.ethCall)
	s.Register("eth_estimateGas", h.ethEstimateGas)
}

func (h *Handler) ethCall(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.CallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}

	out, _, execErr := h.simulate(params.Args, params.Block, h.callGasLimit)
	if execErr != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, execErr.Error())
	}
	if out.VMErr != nil {
		return jsonrpc.ErrResponseWithData(req.ID, jsonrpc.CodeServerError, "execution reverted", hexutil.Encode(out.Data))
	}
	return jsonrpc.OkResponse(req.ID, hexutil.Bytes(out.Data))
}

func (h *Handler) ethEstimateGas(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.CallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}

	limit := h.callGasLimit
	if params.Args.Gas != nil && uint64(*params.Args.Gas) < limit {
		limit = uint64(*params.Args.Gas)
	}

	// Single-pass estimate: run at the full gas limit to check for revert, then return
	// gasUsed + intrinsic. This over-estimates for contracts whose behaviour changes based
	// on available gas (e.g. EIP-1283 stipend checks). A binary search (hi=limit, lo=gasUsed)
	// would find the true minimum, but adds latency and is not required for correctness —
	// wallets and SDKs typically add a 20–25% buffer on top of estimates anyway.
	// TODO: implement binary search if gas-sensitive contracts become common on VeChain.
	out, _, execErr := h.simulate(params.Args, params.Block, limit)
	if execErr != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, execErr.Error())
	}
	if out.VMErr != nil {
		return jsonrpc.ErrResponseWithData(req.ID, jsonrpc.CodeServerError, "execution reverted", hexutil.Encode(out.Data))
	}

	evmGasUsed := limit - out.LeftOverGas

	// PrepareClause does not charge intrinsic gas (tx base + per-clause overhead).
	// Add it explicitly so the estimate matches what the network will deduct.
	var to *thor.Address
	if params.Args.To != nil {
		addr := thor.Address(*params.Args.To)
		to = &addr
	}
	intrinsic, err := tx.IntrinsicGas(tx.NewClause(to).WithData(params.Args.Data))
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}

	// Edge case: if the call uses exactly gasLimit (leftover == 0), this returns
	// callGasLimit + intrinsic — the absolute maximum. The estimate may still be too
	// low for the actual tx, but returning the ceiling is acceptable.
	return jsonrpc.OkResponse(req.ID, hexutil.Uint64(evmGasUsed+intrinsic))
}

func (h *Handler) simulate(args rpc.CallArgs, block rpc.BlockNumberOrHash, gasLimit uint64) (*runtime.Output, *state.State, error) {
	summary, err := ethconvert.ResolveBlockNumberOrHash(block, h.repo)
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
	if args.MaxFeePerGas != nil || args.MaxPriorityFeePerGas != nil {
		maxFee := new(big.Int)
		if args.MaxFeePerGas != nil {
			maxFee = (*big.Int)(args.MaxFeePerGas)
		}
		maxPriority := new(big.Int)
		if args.MaxPriorityFeePerGas != nil {
			maxPriority = (*big.Int)(args.MaxPriorityFeePerGas)
		}
		gasPrice = ethconvert.CalcEffectiveGasPrice(maxFee, maxPriority, header.BaseFee())
	} else if args.GasPrice != nil {
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

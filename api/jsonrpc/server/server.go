// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package server

import "context"

type Server struct {
	registry registry
}

func New() *Server { return &Server{} }

// RegisterName reflects over rcvr's exported methods and registers them under
// namespace: a method Foo becomes callable as "<namespace>_foo" (only the first
// letter is lower-cased, so ChainId -> chainId -> eth_chainId). rcvr is normally
// a pointer, so its pointer-receiver methods are visible. Registering the same
// namespace more than once merges the method sets.
//
// A method is registered only if its signature is one of:
//
//	func (T) Method() error
//	func (T) Method(args...) Result
//	func (T) Method(args...) (Result, error)
//	func (T) Method(ctx context.Context, args...) (Result, error)
//
// Rules:
//   - Only exported methods are considered; the rest are skipped silently.
//   - At most two results; with two, the second must be error. Any other shape
//     is skipped (not reported as an error).
//   - A leading context.Context is injected by the server and is not part of the
//     JSON params — clients must not send it.
//   - The remaining parameters are decoded positionally from the JSON params
//     array. A pointer parameter is optional (nil when omitted); a non-pointer
//     parameter is required and its absence returns -32602 (invalid params).
//   - Params and results must be JSON-(un)marshalable: use *hexutil.Big,
//     hexutil.Bytes, common.Address, etc. — not a raw *big.Int.
//
// It returns an error only when namespace is empty or rcvr exposes no suitable
// method.
//
// Example: the method
//
//	func (a *EthAPI) GetBalance(ctx context.Context, addr common.Address, block *string) (*hexutil.Big, error)
//
// registered via RegisterName("eth", &EthAPI{...}) is exposed as "eth_getBalance",
// ctx auto-injected. addr is non-pointer (required); block is a pointer (optional),
// so clients may send [address, block] or just [address] — omission only from the
// tail. Sending [] omits the required addr and returns -32602.
func (s *Server) RegisterName(namespace string, rcvr any) error {
	return s.registry.registerNamespace(namespace, rcvr)
}

// handleMsg dispatches one request. It returns nil for a notification (a
// request without an "id"), which per JSON-RPC 2.0 must receive no reply.
func (s *Server) handleMsg(ctx context.Context, msg *jsonrpcMessage) *jsonrpcMessage {
	resp := s.dispatch(ctx, msg)
	if len(msg.ID) == 0 {
		return nil
	}
	return resp
}

func (s *Server) dispatch(ctx context.Context, msg *jsonrpcMessage) *jsonrpcMessage {
	if msg.Version != jsonrpcVersion || msg.Method == "" {
		return errorResponse(msg.ID, &jsonError{
			Code:    errcodeInvalidRequest,
			Message: "invalid request",
		})
	}
	cb := s.registry.callback(msg.Method)
	if cb == nil {
		return errorResponse(msg.ID, &jsonError{
			Code:    errcodeMethodNotFound,
			Message: "the method " + msg.Method + " does not exist/is not available",
		})
	}
	args, err := cb.parseArgs(msg.Params)
	if err != nil {
		return errorResponse(msg.ID, toJSONError(err))
	}
	result, err := cb.call(ctx, args)
	if err != nil {
		return errorResponse(msg.ID, toJSONError(err))
	}
	return &jsonrpcMessage{Version: jsonrpcVersion, ID: msg.ID, Result: result}
}

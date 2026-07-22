// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import "context"

type Server struct {
	registry serviceRegistry
}

func NewServer() *Server { return &Server{} }

func (s *Server) RegisterName(namespace string, rcvr interface{}) error {
	return s.registry.registerName(namespace, rcvr)
}

func (s *Server) handleMsg(ctx context.Context, msg *jsonrpcMessage) *jsonrpcMessage {
	if msg.Version != jsonrpcVersion || msg.Method == "" {
		return errorResponse(msg.ID, &jsonError{Code: errcodeInvalidRequest,
			Message: "invalid request"})
	}
	cb := s.registry.callback(msg.Method)
	if cb == nil {
		return errorResponse(msg.ID, &jsonError{Code: errcodeMethodNotFound,
			Message: "the method " + msg.Method + " does not exist/is not available"})
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

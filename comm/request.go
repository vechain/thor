package comm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/thor"
)

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func requestHeader(msg p2p.Msg, s *session) error {
	var id thor.Hash
	if err := msg.Decode(&id); err != nil {
		return errResp(ErrDecode, "%v: %v", msg, err)
	}

	header, err := s.blockChain.GetBlockHeader(id)
	if err != nil && !s.blockChain.IsNotFound(err) {
		return errResp(ErrChain, "%v: %v", msg, err)
	}

	return p2p.Send(s.rw, BlockHeaderMsg, header)
}

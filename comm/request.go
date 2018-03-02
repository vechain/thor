package comm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/p2p"
)

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func handleGetBlockHeadersMsg(msg p2p.Msg, s *session) error {
	// 	// Decode the complex header query
	// 	var query getBlockHeadersData
	// 	if err := msg.Decode(&query); err != nil {
	// 		return errResp(ErrDecode, "%v: %v", msg, err)
	// 	}
	// 	hashMode := query.Origin.ID != (thor.Hash{})

	// 	// Gather headers until the fetch or network limits is reached
	// 	var (
	// 		bytes   float64
	// 		headers []*block.Header
	// 		unknown bool
	// 	)
	// 	for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < downloader.MaxHeaderFetch {
	// 		// Retrieve the next header satisfying the query
	// 		var (
	// 			blk *block.Block
	// 			err error
	// 		)
	// 		if hashMode {
	// 			blk, err = s.blockChain.GetBlock(query.Origin.ID)
	// 		} else {
	// 			blk, err = s.blockChain.GetBlockByNumber(query.Origin.Number)
	// 		}
	// 		if err != nil {
	// 			if !s.blockChain.IsNotFound(err) {
	// 				return errResp(ErrDecode, "%v: %v", msg, err)
	// 			}
	// 			break
	// 		}
	// 		origin := blk.Header()
	// 		headers = append(headers, origin)
	// 		bytes += estHeaderRlpSize

	// 		// Advance to the next header of the query
	// 		switch {
	// 		case query.Origin.ID != (thor.Hash{}) && query.Reverse:
	// 			// Hash based traversal towards the genesis block
	// 			for i := 0; i < int(query.Skip)+1; i++ {
	// 				if header, err := s.blockChain.GetBlockHeader(query.Origin.ID); err == nil {
	// 					query.Origin.ID = header.ParentID()
	// 				} else {
	// 					if !s.blockChain.IsNotFound(err) {
	// 						return errResp(ErrDecode, "%v: %v", msg, err)
	// 					}
	// 					unknown = true
	// 					break
	// 				}
	// 			}
	// 		case query.Origin.ID != (thor.Hash{}) && !query.Reverse:
	// 			// Hash based traversal towards the leaf block
	// 			var (
	// 				current = origin.Number()
	// 				next    = current + query.Skip + 1
	// 			)
	// 			if next <= current {
	// 				infos, _ := json.MarshalIndent(s.p.Info(), "", "  ")
	// 				s.p.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
	// 				unknown = true
	// 			} else {
	// 				if block, err := s.blockChain.GetBlockByNumber(next); err != nil {
	// 					if !s.blockChain.IsNotFound(err) {
	// 						return errResp(ErrDecode, "%v: %v", msg, err)
	// 					}
	// 					unknown = true
	// 				} else {
	// 					if s.blockChain.GetBlockHashesFromHash(header.Hash(), query.Skip+1)[query.Skip] == query.Origin.Hash {
	// 						query.Origin.ID = block.Header().ID()
	// 					} else {
	// 						unknown = true
	// 					}
	// 				}
	// 			}
	// 		case query.Reverse:
	// 			// Number based traversal towards the genesis block
	// 			if query.Origin.Number >= query.Skip+1 {
	// 				query.Origin.Number -= query.Skip + 1
	// 			} else {
	// 				unknown = true
	// 			}
	// 		case !query.Reverse:
	// 			// Number based traversal towards the leaf block
	// 			query.Origin.Number += query.Skip + 1
	// 		}
	// 	}
	//return s.sendBlockHeaders(headers)
	return nil
}

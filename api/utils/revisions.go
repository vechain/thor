package utils

import (
	"math"
	"strconv"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type Revision = interface{}

func GetSummary(revision Revision, repo *chain.Repository, bft bft.Finalizer) (s *chain.BlockSummary, err error) {
	var id thor.Bytes32
	switch revision := revision.(type) {
	case thor.Bytes32:
		id = revision
	case uint32:
		id, err = repo.NewBestChain().GetBlockID(revision)
		if err != nil {
			return
		}
	case string:
		id = bft.Finalized()
	default:
		id = repo.BestBlockSummary().Header.ID()
	}
	summary, err := repo.GetBlockSummary(id)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

// ParseRevision parses a query parameter into a block number or block ID.
func ParseRevision(revision string) (Revision, error) {
	if revision == "" || revision == "best" {
		return nil, nil
	}
	if revision == "finalized" {
		return revision, nil
	}
	if len(revision) == 66 || len(revision) == 64 {
		blockID, err := thor.ParseBytes32(revision)
		if err != nil {
			return nil, err
		}
		return blockID, nil
	}
	n, err := strconv.ParseUint(revision, 0, 0)
	if err != nil {
		return nil, err
	}
	if n > math.MaxUint32 {
		return nil, errors.New("block number out of max uint32")
	}
	return uint32(n), err
}

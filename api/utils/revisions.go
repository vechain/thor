package utils

import (
	"math"
	"strconv"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type bftEngine interface {
	Finalized() thor.Bytes32
}

type RevisionHandler struct {
	repo *chain.Repository
	bft  bftEngine
}

type Revision = interface{}

func NewRevisionHandler(repo *chain.Repository, bft bftEngine) *RevisionHandler {
	return &RevisionHandler{
		repo: repo,
		bft:  bft,
	}
}

func (h *RevisionHandler) GetSummary(revision Revision) (s *chain.BlockSummary, err error) {
	var id thor.Bytes32
	switch revision := revision.(type) {
	case thor.Bytes32:
		id = revision
	case uint32:
		id, err = h.repo.NewBestChain().GetBlockID(revision)
		if err != nil {
			return
		}
	case string:
		id = h.bft.Finalized()
	default:
		id = h.repo.BestBlockSummary().Header.ID()
	}
	summary, err := h.repo.GetBlockSummary(id)
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

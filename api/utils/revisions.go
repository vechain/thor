// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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

// ParseCodeCallRevision parses the revision query parameter for endpoints that may be estimating gas for new transactions.
func ParseCodeCallRevision(revision string) (Revision, bool, error) {
	if revision == "next" || revision == "" || revision == "best" {
		return nil, true, nil
	}

	res, err := ParseRevision(revision)
	return res, false, err
}

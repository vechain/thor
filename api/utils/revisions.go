// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package utils

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

const (
	revBest      int64 = -1
	revFinalized int64 = -2
	revNext      int64 = -3
	revJustified int64 = -4
)

type Revision struct {
	val interface{}
}

func (rev *Revision) IsNext() bool {
	return rev.val == revNext
}

func NewRevision(val interface{}) *Revision {
	return &Revision{val}
}

// ParseRevision parses a query parameter into a block number or block ID.
func ParseRevision(revision string, allowNext bool) (*Revision, error) {
	if revision == "" || revision == "best" {
		return &Revision{revBest}, nil
	}

	if revision == "finalized" {
		return &Revision{revFinalized}, nil
	}

	if revision == "justified" {
		return &Revision{revJustified}, nil
	}

	if revision == "next" {
		if !allowNext {
			return nil, errors.New("invalid revision: next is not allowed")
		}
		return &Revision{revNext}, nil
	}

	if len(revision) == 66 || len(revision) == 64 {
		blockID, err := thor.ParseBytes32(revision)
		if err != nil {
			return nil, err
		}
		return &Revision{blockID}, nil
	}
	n, err := strconv.ParseUint(revision, 0, 0)
	if err != nil {
		return nil, err
	}
	if n > math.MaxUint32 {
		return nil, errors.New("block number out of max uint32")
	}
	return &Revision{uint32(n)}, err
}

// GetSummary returns the block summary for the given revision,
// revision required to be a deterministic block other than "next".
func GetSummary(rev *Revision, repo *chain.Repository, bft bft.Committer) (sum *chain.BlockSummary, err error) {
	var id thor.Bytes32
	switch rev := rev.val.(type) {
	case thor.Bytes32:
		id = rev
	case uint32:
		id, err = repo.NewBestChain().GetBlockID(rev)
		if err != nil {
			return
		}
	case int64:
		switch rev {
		case revBest:
			id = repo.BestBlockSummary().Header.ID()
		case revFinalized:
			id = bft.Finalized()
		case revJustified:
			id, err = bft.Justified()
			if err != nil {
				return nil, err
			}
		}
	}
	if id.IsZero() {
		return nil, errors.New("invalid revision")
	}
	summary, err := repo.GetBlockSummary(id)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

// GetSummaryAndState returns the block summary and state for the given revision,
// this function supports the "next" revision.
func GetSummaryAndState(rev *Revision, repo *chain.Repository, bft bft.Committer, stater *state.Stater) (*chain.BlockSummary, *state.State, error) {
	if rev.IsNext() {
		best := repo.BestBlockSummary()

		// here we create a fake(no signature) "next" block header which reused most part of the parent block
		// but set the timestamp and number to the next block. The following parameters will be used in the evm
		// number, timestamp, total score, gas limit, beneficiary and "signer"
		// since the fake block is not signed, the signer is the zero address, it is important that the subsequent
		// call to header.Signer(), the error should be ignored.
		builder := new(block.Builder).
			ParentID(best.Header.ID()).
			Timestamp(best.Header.Timestamp() + thor.BlockInterval).
			TotalScore(best.Header.TotalScore()).
			GasLimit(best.Header.GasLimit()).
			GasUsed(best.Header.GasUsed()).
			Beneficiary(best.Header.Beneficiary()).
			StateRoot(best.Header.StateRoot()).
			ReceiptsRoot(best.Header.ReceiptsRoot()).
			TransactionFeatures(best.Header.TxsFeatures()).
			Alpha(best.Header.Alpha())

		// here we skipped the block's tx list thus header.txRoot will be an empty root
		// since txRoot won't be supplied into the evm, it's safe to skip it.
		if best.Header.COM() {
			builder.COM()
		}
		mocked := builder.Build()

		// state is also reused from the parent block
		st := stater.NewState(best.Header.StateRoot(), best.Header.Number(), best.Conflicts, best.SteadyNum)

		// rebuild the block summary with the next header (mocked) AND the best block status
		return &chain.BlockSummary{
			Header:    mocked.Header(),
			Txs:       best.Txs,
			Size:      uint64(mocked.Size()),
			Conflicts: best.Conflicts,
			SteadyNum: best.SteadyNum,
		}, st, nil
	}
	sum, err := GetSummary(rev, repo, bft)
	if err != nil {
		return nil, nil, err
	}

	st := stater.NewState(sum.Header.StateRoot(), sum.Header.Number(), sum.Conflicts, sum.SteadyNum)
	return sum, st, nil
}

func (r *Revision) MarshalJSON() ([]byte, error) {
	switch v := r.val.(type) {
	case int64:
		switch v {
		case revBest:
			return json.Marshal("best")
		case revFinalized:
			return json.Marshal("finalized")
		case revNext:
			return json.Marshal("next")
		case revJustified:
			return json.Marshal("justified")
		}
	case thor.Bytes32:
		return json.Marshal(v.String())
	case uint32:
		return json.Marshal(v)
	}
	return nil, errors.New("unsupported type for Revision")
}

func (r *Revision) UnmarshalJSON(data []byte) error {
	var n uint32
	if err := json.Unmarshal(data, &n); err == nil {
		r.val = n
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		switch s {
		case "best":
			r.val = revBest
		case "finalized":
			r.val = revFinalized
		case "next":
			r.val = revNext
		case "justified":
			r.val = revJustified
		default:
			if len(s) == 66 || len(s) == 64 {
				blockID, err := thor.ParseBytes32(s)
				if err != nil {
					return err
				}
				r.val = blockID
			} else {
				return errors.New("invalid string for Revision")
			}
		}
		return nil
	}

	return errors.New("unsupported type for Revision")
}

// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type WeakSubjectivityCheckpoint struct {
	ID           thor.Bytes32 `json:"id"`
	Number       uint32       `json:"number"`
	TxsRoot      thor.Bytes32 `json:"txsRoot"`
	StateRoot    thor.Bytes32 `json:"stateRoot"`
	ReceiptsRoot thor.Bytes32 `json:"receiptsRoot"`
}

type wspBlockSummary struct {
	Number        uint32                `json:"number"`
	ID            thor.Bytes32          `json:"id"`
	Size          uint32                `json:"size"`
	ParentID      thor.Bytes32          `json:"parentID"`
	Timestamp     uint64                `json:"timestamp"`
	GasLimit      uint64                `json:"gasLimit"`
	Beneficiary   thor.Address          `json:"beneficiary"`
	GasUsed       uint64                `json:"gasUsed"`
	TotalScore    uint64                `json:"totalScore"`
	TxsRoot       thor.Bytes32          `json:"txsRoot"`
	TxsFeatures   uint32                `json:"txsFeatures"`
	StateRoot     thor.Bytes32          `json:"stateRoot"`
	ReceiptsRoot  thor.Bytes32          `json:"receiptsRoot"`
	COM           bool                  `json:"com"`
	Signer        thor.Address          `json:"signer"`
	IsTrunk       bool                  `json:"isTrunk"`
	IsFinalized   bool                  `json:"isFinalized"`
	BaseFeePerGas *math.HexOrDecimal256 `json:"baseFeePerGas,omitempty"`
}

type wspResponse struct {
	wspBlockSummary
	Transactions []thor.Bytes32 `json:"transactions"`
}

type WeakSubjectivityChecker struct {
	repo           *chain.Repository
	bft            bft.Committer
	client         *http.Client
	wspProviderURL string
}

func NewWeakSubjectivityChecker(repo *chain.Repository, bft bft.Committer, wscURL string) *WeakSubjectivityChecker {
	return &WeakSubjectivityChecker{
		repo:           repo,
		bft:            bft,
		client:         &http.Client{},
		wspProviderURL: wscURL,
	}
}

func ValidateWeakSubjectivityCheckpoint(blk *block.Block, checkpoint *WeakSubjectivityCheckpoint) (bool, error) {
	now := uint64(time.Now().Unix())
	checkpointTimestamp := blk.Header().Timestamp()

	if now <= checkpointTimestamp {
		return false, fmt.Errorf("checkpoint is in the future")
	}

	if blk.Header().ID() != checkpoint.ID {
		return false, fmt.Errorf("block id mismatch")
	}

	if blk.Header().Number() != checkpoint.Number {
		return false, fmt.Errorf("number mismatch")
	}

	if blk.Header().TxsRoot() != checkpoint.TxsRoot {
		return false, fmt.Errorf("txs root mismatch")
	}

	if blk.Header().StateRoot() != checkpoint.StateRoot {
		return false, fmt.Errorf("state root mismatch")
	}

	if blk.Header().ReceiptsRoot() != checkpoint.ReceiptsRoot {
		return false, fmt.Errorf("receipts root mismatch")
	}

	return true, nil
}

func (w *WeakSubjectivityChecker) LatestFinalizedBlock() (*block.Block, error) {
	if w.bft == nil {
		return nil, fmt.Errorf("bft engine is not set")
	}
	finalizedID := w.bft.Finalized()
	blk, err := w.repo.GetBlock(finalizedID)
	if err != nil {
		return nil, fmt.Errorf("failed to get finalized block: %w", err)
	}
	return blk, nil
}

func (w *WeakSubjectivityChecker) PerformCheck(ctx context.Context) error {
	// Retry fetch with exponential backoff (3 attempts, starting at 1s up to 8s)
	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var checkpoint *WeakSubjectivityCheckpoint
	if err := retryWithBackoff(fetchCtx, 3, time.Second, 8*time.Second, func(ctx context.Context) error {
		callCtx, callCancel := context.WithTimeout(ctx, 10*time.Second)
		defer callCancel()
		cp, err := w.fetchWeakSubjectivityCheckpoint(callCtx, w.wspProviderURL)
		if err != nil {
			return err
		}
		checkpoint = cp
		return nil
	}); err != nil {
		return err
	}

	// We compare the internal latest finalized block, provided by the bft engine
	// with the received checkpoint.
	internalFinalizedBlk, err := w.LatestFinalizedBlock()
	if err != nil {
		return err
	}

	if err := w.checkFinalizedAndCheckpoint(internalFinalizedBlk, checkpoint); err != nil {
		return err
	}
	return nil
}

func (w *WeakSubjectivityChecker) checkFinalizedAndCheckpoint(finalizedBlk *block.Block, checkpoint *WeakSubjectivityCheckpoint) error {
	header := finalizedBlk.Header()
	if header.ID() != checkpoint.ID {
		return fmt.Errorf("finalized block ID does not match checkpoint")
	}
	if header.Number() != checkpoint.Number {
		return fmt.Errorf("finalized block number does not match checkpoint")
	}
	if header.TxsRoot() != checkpoint.TxsRoot {
		return fmt.Errorf("finalized block TxsRoot does not match checkpoint")
	}
	if header.StateRoot() != checkpoint.StateRoot {
		return fmt.Errorf("finalized block StateRoot does not match checkpoint")
	}
	if header.ReceiptsRoot() != checkpoint.ReceiptsRoot {
		return fmt.Errorf("finalized block ReceiptsRoot does not match checkpoint")
	}
	return nil
}

func (w *WeakSubjectivityChecker) fetchWeakSubjectivityCheckpoint(ctx context.Context, url string) (*WeakSubjectivityCheckpoint, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %v", resp.Status)
	}

	var r wspResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode wsp response: %w", err)
	}

	if !r.IsFinalized {
		return nil, fmt.Errorf("checkpoint is not finalized")
	}

	return &WeakSubjectivityCheckpoint{
		ID:           r.ID,
		Number:       r.Number,
		TxsRoot:      r.TxsRoot,
		StateRoot:    r.StateRoot,
		ReceiptsRoot: r.ReceiptsRoot,
	}, nil
}

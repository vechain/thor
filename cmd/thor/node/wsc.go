// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

const (
	wscSafeRange    = time.Hour
	wscFetchTimeout = 15 * time.Second
	wscMaxBodySize  = 16 * 1024 * 1024
)

// finalizedStatus loads the finalized block and computes its safe-range status.
func (n *Node) finalizedStatus() (thor.Bytes32, bool, time.Duration, error) {
	summary, err := n.repo.GetBlockSummary(n.bft.Finalized())
	if err != nil {
		return thor.Bytes32{}, false, 0, err
	}
	id := summary.Header.ID()
	safe, age, err := finalizedInSafeRange(time.Now(), summary)
	return id, safe, age, err
}

// shouldCheckWeakSubjectivityCheckpoint only enables WSC when a provider URL is configured
// and the local finalized block is stale enough to warrant external verification.
func (n *Node) shouldCheckWeakSubjectivityCheckpoint() (bool, error) {
	if n.options.WSCProviderURL == "" {
		logger.Debug("weak subjectivity checkpoint disabled: provider url not set")
		return false, nil
	}

	id, safe, age, err := n.finalizedStatus()
	if err != nil {
		return false, err
	}
	if safe {
		logger.Info("weak subjectivity checkpoint skipped", "finalized", shortID(id), "age", age)
		return false, nil
	}

	logger.Info("weak subjectivity checkpoint required", "finalized", shortID(id), "age", age, "provider", n.options.WSCProviderURL)
	return true, nil
}

// verifyWeakSubjectivityCheckpoint fetches the remote checkpoint after sync and
// enforces both the safe-range constraint and an exact ID match against the local finalized block.
func (n *Node) verifyWeakSubjectivityCheckpoint(ctx context.Context) error {
	checkpoint, err := fetchWSCheckpoint(ctx, n.options.WSCProviderURL)
	if err != nil {
		return err
	}

	id, safe, age, err := n.finalizedStatus()
	if err != nil {
		return err
	}
	if !safe {
		return fmt.Errorf("finalized block outside safe range (age %s)", age)
	}
	if checkpoint != id {
		return fmt.Errorf("checkpoint mismatch: checkpoint=%s finalized=%s", checkpoint, id)
	}

	logger.Info("weak subjectivity checkpoint verified", "finalized", shortID(id))
	return nil
}

// finalizedInSafeRange returns false (with error) if the finalized timestamp is ahead of local time.
func finalizedInSafeRange(now time.Time, summary *chain.BlockSummary) (bool, time.Duration, error) {
	age, err := finalizedAge(now, summary)
	if err != nil {
		return false, 0, err
	}
	return age <= wscSafeRange, age, nil
}

// finalizedAge fails if the finalized timestamp is ahead of local time.
func finalizedAge(now time.Time, summary *chain.BlockSummary) (time.Duration, error) {
	blockTime := time.Unix(int64(summary.Header.Timestamp()), 0)
	if blockTime.After(now) {
		return 0, fmt.Errorf("finalized block timestamp %s is in the future", blockTime)
	}
	return now.Sub(blockTime), nil
}

// fetchWSCheckpoint performs a bounded HTTP GET and expects a JSON payload with an id field.
func fetchWSCheckpoint(ctx context.Context, url string) (thor.Bytes32, error) {
	if url == "" {
		return thor.Bytes32{}, fmt.Errorf("missing checkpoint url")
	}

	reqCtx, cancel := context.WithTimeout(ctx, wscFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return thor.Bytes32{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return thor.Bytes32{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return thor.Bytes32{}, fmt.Errorf("unexpected response status %s", resp.Status)
	}

	var payload struct {
		ID *thor.Bytes32 `json:"id"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, wscMaxBodySize))
	if err := decoder.Decode(&payload); err != nil {
		return thor.Bytes32{}, fmt.Errorf("decode checkpoint response: %w", err)
	}
	if payload.ID == nil {
		return thor.Bytes32{}, fmt.Errorf("checkpoint response missing id")
	}
	return *payload.ID, nil
}

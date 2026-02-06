// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

func summaryWithTimestamp(ts time.Time) *chain.BlockSummary {
	blk := (&block.Builder{}).Timestamp(uint64(ts.Unix())).Build()
	return &chain.BlockSummary{Header: blk.Header()}
}

func newCheckpointServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func newTestNode(t *testing.T, launchTime uint64, wscURL string) *Node {
	t.Helper()
	forkConfig := thor.SoloFork
	chain, err := testchain.NewIntegrationTestChain(genesis.DevConfig{
		ForkConfig: &forkConfig,
		LaunchTime: launchTime,
	}, 180)
	require.NoError(t, err)

	return &Node{
		repo:    chain.Repo(),
		bft:     chain.Engine(),
		options: Options{WSCProviderURL: wscURL},
	}
}

func TestFinalizedInSafeRange(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	cases := []struct {
		name      string
		timestamp time.Time
		expected  bool
		wantErr   bool
	}{
		{"future timestamp", now.Add(10 * time.Second), false, true},
		{"exactly safe range", now.Add(-wscSafeRange), true, false},
		{"just outside safe range", now.Add(-wscSafeRange - time.Second), false, false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			summary := summaryWithTimestamp(tt.timestamp)
			safe, age, err := finalizedInSafeRange(now, summary)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, safe)
			if tt.expected {
				require.LessOrEqual(t, age, wscSafeRange)
			}
		})
	}
}

func TestFinalizedAge(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	future := summaryWithTimestamp(now.Add(15 * time.Second))
	_, err := finalizedAge(now, future)
	require.Error(t, err)

	past := summaryWithTimestamp(now.Add(-2 * time.Minute))
	age, err := finalizedAge(now, past)
	require.NoError(t, err)
	require.Equal(t, 2*time.Minute, age)
}

func TestFetchWSCheckpoint(t *testing.T) {
	id := thor.MustParseBytes32("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20")

	t.Run("success", func(t *testing.T) {
		body := fmt.Sprintf(`{"id":"%s"}`, id.String())
		server := newCheckpointServer(http.StatusOK, body)
		defer server.Close()

		got, err := fetchWSCheckpoint(context.Background(), server.URL)
		require.NoError(t, err)
		require.Equal(t, id, got)
	})

	t.Run("missing url", func(t *testing.T) {
		_, err := fetchWSCheckpoint(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("non-2xx status", func(t *testing.T) {
		server := newCheckpointServer(http.StatusInternalServerError, `{"id":"0x00"}`)
		defer server.Close()

		_, err := fetchWSCheckpoint(context.Background(), server.URL)
		require.ErrorContains(t, err, "unexpected response status")
	})

	t.Run("invalid json", func(t *testing.T) {
		server := newCheckpointServer(http.StatusOK, "not-json")
		defer server.Close()

		_, err := fetchWSCheckpoint(context.Background(), server.URL)
		require.ErrorContains(t, err, "decode checkpoint response")
	})

	t.Run("missing id", func(t *testing.T) {
		server := newCheckpointServer(http.StatusOK, `{}`)
		defer server.Close()

		_, err := fetchWSCheckpoint(context.Background(), server.URL)
		require.ErrorContains(t, err, "missing id")
	})

	t.Run("invalid id", func(t *testing.T) {
		server := newCheckpointServer(http.StatusOK, `{"id":"0x1234"}`)
		defer server.Close()

		_, err := fetchWSCheckpoint(context.Background(), server.URL)
		require.ErrorContains(t, err, "decode checkpoint response")
	})
}

func TestPrepareWeakSubjectivity(t *testing.T) {
	now := time.Now()

	t.Run("safe range", func(t *testing.T) {
		node := newTestNode(t, uint64(now.Add(-10*time.Second).Unix()), "http://example.test")
		required, err := node.shouldCheckWeakSubjectivityCheckpoint()
		require.NoError(t, err)
		require.False(t, required)
	})

	t.Run("out of range without url", func(t *testing.T) {
		node := newTestNode(t, uint64(now.Add(-2*time.Hour).Unix()), "")
		required, err := node.shouldCheckWeakSubjectivityCheckpoint()
		require.NoError(t, err)
		require.False(t, required)
	})

	t.Run("future timestamp without url", func(t *testing.T) {
		node := newTestNode(t, uint64(now.Add(2*time.Hour).Unix()), "")
		required, err := node.shouldCheckWeakSubjectivityCheckpoint()
		require.NoError(t, err)
		require.False(t, required)
	})

	t.Run("out of range with url", func(t *testing.T) {
		node := newTestNode(t, uint64(now.Add(-2*time.Hour).Unix()), "http://example.test")
		required, err := node.shouldCheckWeakSubjectivityCheckpoint()
		require.NoError(t, err)
		require.True(t, required)
	})
}

func TestVerifyWeakSubjectivity(t *testing.T) {
	now := time.Now()

	t.Run("match", func(t *testing.T) {
		node := newTestNode(t, uint64(now.Add(-10*time.Second).Unix()), "")
		finalizedID := node.bft.Finalized()
		server := newCheckpointServer(http.StatusOK, fmt.Sprintf(`{"id":"%s"}`, finalizedID.String()))
		defer server.Close()

		node.options.WSCProviderURL = server.URL
		require.NoError(t, node.verifyWeakSubjectivityCheckpoint(context.Background()))
	})

	t.Run("mismatch", func(t *testing.T) {
		node := newTestNode(t, uint64(now.Add(-10*time.Second).Unix()), "")
		finalizedID := node.bft.Finalized()
		badID := finalizedID
		badID[31] ^= 0xff
		server := newCheckpointServer(http.StatusOK, fmt.Sprintf(`{"id":"%s"}`, badID.String()))
		defer server.Close()

		node.options.WSCProviderURL = server.URL
		err := node.verifyWeakSubjectivityCheckpoint(context.Background())
		require.ErrorContains(t, err, "checkpoint mismatch")
	})

	t.Run("outside safe range", func(t *testing.T) {
		node := newTestNode(t, uint64(now.Add(-2*time.Hour).Unix()), "")
		finalizedID := node.bft.Finalized()
		server := newCheckpointServer(http.StatusOK, fmt.Sprintf(`{"id":"%s"}`, finalizedID.String()))
		defer server.Close()

		node.options.WSCProviderURL = server.URL
		err := node.verifyWeakSubjectivityCheckpoint(context.Background())
		require.ErrorContains(t, err, "outside safe range")
	})
}

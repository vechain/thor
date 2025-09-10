// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func TestValidateWeakSubjectivityCheckpoint(t *testing.T) {
	blk := new(block.Builder).Build()
	checkpoint := &WeakSubjectivityCheckpoint{
		ID:           blk.Header().ID(),
		Number:       blk.Header().Number(),
		TxsRoot:      blk.Header().TxsRoot(),
		StateRoot:    blk.Header().StateRoot(),
		ReceiptsRoot: blk.Header().ReceiptsRoot(),
	}
	valid, err := ValidateWeakSubjectivityCheckpoint(blk, checkpoint)
	assert.True(t, valid)
	assert.NoError(t, err)
}

func TestFetchWeakSubjectivityCheckpoint_Success(t *testing.T) {
	id := thor.MustParseBytes32("0x" + strings.Repeat("11", 32))
	txsRoot := thor.MustParseBytes32("0x" + strings.Repeat("22", 32))
	stateRoot := thor.MustParseBytes32("0x" + strings.Repeat("33", 32))
	receiptsRoot := thor.MustParseBytes32("0x" + strings.Repeat("44", 32))
	num := uint32(12345)

	body := fmt.Sprintf(`{
		"number": %d,
		"id": %q,
		"txsRoot": %q,
		"stateRoot": %q,
		"receiptsRoot": %q,
		"isFinalized": true,
		"transactions": []
	}`, num, id.String(), txsRoot.String(), stateRoot.String(), receiptsRoot.String())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	wsc := &WeakSubjectivityChecker{client: &http.Client{}}
	got, err := wsc.fetchWeakSubjectivityCheckpoint(context.Background(), srv.URL)
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, num, got.Number)
	assert.Equal(t, txsRoot, got.TxsRoot)
	assert.Equal(t, stateRoot, got.StateRoot)
	assert.Equal(t, receiptsRoot, got.ReceiptsRoot)
}

func TestFetchWeakSubjectivityCheckpoint_NotFinalized(t *testing.T) {
	id := thor.MustParseBytes32("0x" + strings.Repeat("aa", 32))

	body := fmt.Sprintf(`{
		"number": %d,
		"id": %q,
		"isFinalized": false
	}`, 100, id.String())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	wsc := &WeakSubjectivityChecker{client: &http.Client{}}
	got, err := wsc.fetchWeakSubjectivityCheckpoint(context.Background(), srv.URL)
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint is not finalized")
}

func TestFetchWeakSubjectivityCheckpoint_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	wsc := &WeakSubjectivityChecker{client: &http.Client{}}
	got, err := wsc.fetchWeakSubjectivityCheckpoint(context.Background(), srv.URL)
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500 Internal Server Error")
}

func TestFetchWeakSubjectivityCheckpoint_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not-json}"))
	}))
	defer srv.Close()

	wsc := &WeakSubjectivityChecker{client: &http.Client{}}
	got, err := wsc.fetchWeakSubjectivityCheckpoint(context.Background(), srv.URL)
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode wsp response")
}

func TestFetchWeakSubjectivityCheckpoint_InvalidID(t *testing.T) {
	body := `{
		"number": 77,
		"id": "0x1234",
		"isFinalized": true,
		"txsRoot": "0x` + strings.Repeat("55", 32) + `",
		"stateRoot": "0x` + strings.Repeat("66", 32) + `",
		"receiptsRoot": "0x` + strings.Repeat("77", 32) + `"
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	wsc := &WeakSubjectivityChecker{client: &http.Client{}}
	got, err := wsc.fetchWeakSubjectivityCheckpoint(context.Background(), srv.URL)
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode wsp response")
}

func TestCheckFinalizedAndCheckpoint_Positive(t *testing.T) {
	blk := new(block.Builder).Build()
	checkpoint := &WeakSubjectivityCheckpoint{
		ID:           blk.Header().ID(),
		Number:       blk.Header().Number(),
		TxsRoot:      blk.Header().TxsRoot(),
		StateRoot:    blk.Header().StateRoot(),
		ReceiptsRoot: blk.Header().ReceiptsRoot(),
	}
	wsc := &WeakSubjectivityChecker{}
	require.NotPanics(t, func() {
		wsc.checkFinalizedAndCheckpoint(blk, checkpoint)
	})
}

func TestCheckFinalizedAndCheckpoint_Negative(t *testing.T) {
	blk := new(block.Builder).Build()
	base := &WeakSubjectivityCheckpoint{
		ID:           blk.Header().ID(),
		Number:       blk.Header().Number(),
		TxsRoot:      blk.Header().TxsRoot(),
		StateRoot:    blk.Header().StateRoot(),
		ReceiptsRoot: blk.Header().ReceiptsRoot(),
	}
	wsc := &WeakSubjectivityChecker{}

	cases := []struct {
		name   string
		mutate func(*WeakSubjectivityCheckpoint)
		errMsg string
	}{
		{
			"ID mismatch",
			func(cp *WeakSubjectivityCheckpoint) { cp.ID[0] ^= 0xFF },
			"finalized block ID does not match checkpoint",
		},
		{
			"Number mismatch",
			func(cp *WeakSubjectivityCheckpoint) { cp.Number++ },
			"finalized block number does not match checkpoint",
		},
		{
			"TxsRoot mismatch",
			func(cp *WeakSubjectivityCheckpoint) { cp.TxsRoot[0] ^= 0xFF },
			"finalized block TxsRoot does not match checkpoint",
		},
		{
			"StateRoot mismatch",
			func(cp *WeakSubjectivityCheckpoint) { cp.StateRoot[0] ^= 0xFF },
			"finalized block StateRoot does not match checkpoint",
		},
		{
			"ReceiptsRoot mismatch",
			func(cp *WeakSubjectivityCheckpoint) { cp.ReceiptsRoot[0] ^= 0xFF },
			"finalized block ReceiptsRoot does not match checkpoint",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cp := *base // copy
			c.mutate(&cp)
			err := wsc.checkFinalizedAndCheckpoint(blk, &cp)
			require.Error(t, err)
			assert.EqualError(t, err, c.errMsg)
		})
	}
}

func TestLatestFinalizedBlock_Positive(t *testing.T) {
	memDB := muxdb.NewMem()
	g := genesis.NewDevnet()
	genesis, _, _, _ := g.Build(state.NewStater(memDB))

	repo, err := chain.NewRepository(memDB, genesis)
	require.NoError(t, err)

	testParentID := genesis.Header().ID()
	testBlk := new(block.Builder).ParentID(testParentID).Build()
	repo.AddBlock(testBlk, nil, 0, false)

	bft := &mockBFT{finalized: testBlk.Header().ID()}
	wsc := &WeakSubjectivityChecker{repo: repo, bft: bft}
	got, err := wsc.LatestFinalizedBlock()
	assert.NoError(t, err)
	assert.Equal(t, testBlk.Header().ID(), got.Header().ID())
}

func TestLatestFinalizedBlock_Negative(t *testing.T) {
	wsc := &WeakSubjectivityChecker{}
	got, err := wsc.LatestFinalizedBlock()
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bft engine is not set")

	memDB := muxdb.NewMem()
	g := genesis.NewDevnet()
	genesis, _, _, _ := g.Build(state.NewStater(memDB))

	repo, err := chain.NewRepository(memDB, genesis)
	require.NoError(t, err)
	bft := &mockBFT{finalized: thor.MustParseBytes32("0x" + strings.Repeat("22", 32))}
	wsc = &WeakSubjectivityChecker{repo: repo, bft: bft}
	got, err = wsc.LatestFinalizedBlock()
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get finalized block")
}

type mockBFT struct {
	finalized thor.Bytes32
}

func (m *mockBFT) Finalized() thor.Bytes32          { return m.finalized }
func (m *mockBFT) Justified() (thor.Bytes32, error) { return thor.Bytes32{}, nil }

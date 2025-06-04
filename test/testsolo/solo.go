// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testsolo

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

const (
	gasLimit = 30_000_000
)

type Solo struct {
	Chain     *testchain.Chain
	Shutdown  func() error
	APIServer *httptest.Server
	Solo      *solo.Solo
}

func NewSolo(chain *testchain.Chain) (*Solo, error) {
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1) // Buffered channel to avoid goroutine leak

	mempool := txpool.New(
		chain.Repo(),
		chain.Stater(),
		txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute},
		chain.GetForkConfig(),
	)

	apiHandler, apiCloser := api.New(
		chain.Repo(),
		chain.Stater(),
		mempool,
		chain.LogDB(),
		bft.NewMockedEngine(chain.Repo().GenesisBlock().Header().ID()),
		&solo.Communicator{},
		chain.GetForkConfig(),
		api.Config{
			AllowedOrigins: "*",
			BacktraceLimit: 100,
			CallGasLimit:   40_000_000,
			SkipLogs:       false,
			Fees: fees.Config{
				APIBacktraceLimit:          100,
				FixedCacheSize:             1024,
				PriorityIncreasePercentage: 5,
			},
			EnableReqLogger:  &atomic.Bool{},
			LogsLimit:        100,
			AllowedTracers:   []string{"all"},
			EnableDeprecated: true,
			SoloMode:         true,
			EnableTxpool:     true,
		},
	)

	thorSolo := solo.New(
		chain.Repo(),
		chain.Stater(),
		chain.LogDB(),
		mempool,
		chain.GetForkConfig(),
		solo.Options{
			GasLimit:         gasLimit,
			SkipLogs:         false,
			MinTxPriorityFee: 0,
			OnDemand:         true,
			BlockInterval:    thor.BlockInterval,
		},
	)

	go func() {
		errChan <- thorSolo.Run(ctx) // Capture the error
	}()

	// Check if solo started successfully
	select {
	case err := <-errChan:
		cancel() // Clean up if solo failed to start
		return nil, fmt.Errorf("solo failed to start: %w", err)
	case <-time.After(time.Second): // Give solo a second to start
		// Solo started successfully
	}

	apiServer := httptest.NewServer(apiHandler)
	cleanup := func() error {
		cancel()          // Stop solo
		apiCloser()       // Close API
		apiServer.Close() // Close server

		// Wait for solo to stop and check for errors
		if err := <-errChan; !errors.Is(err, context.Canceled) && err != nil {
			return fmt.Errorf("solo stopped with error: %w", err)
		}
		return nil
	}

	return &Solo{Chain: chain, Solo: thorSolo, APIServer: apiServer, Shutdown: cleanup}, nil
}

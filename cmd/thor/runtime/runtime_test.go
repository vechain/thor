// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package runtime

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/blocks"
)

func TestStart(t *testing.T) {
	baseArgs := []string{os.Args[0]}

	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "default",
			args: []string{"--network", "main"},
		},
		{
			name: "solo",
			args: []string{"solo"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Cleanup(func() {
				os.RemoveAll(dir)
			})
			apiAddr := newApiAddr(t)

			// Combine baseArgs with the test case-specific arguments
			args := append(baseArgs, tc.args...)
			args = append(args, "--data-dir", dir, "--api-addr", apiAddr, "--skip-logs")
			go Start(args)

			assert.NoError(t, waitForServer(apiAddr))
		})
	}
}

func newApiAddr(t *testing.T) string {
	// Create a listener on any available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Extract the port number from the listener's address
	addr := listener.Addr().(*net.TCPAddr)
	return addr.String()
}

func waitForServer(apiAddr string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	timeout := time.NewTimer(5 * time.Second)

	for {
		select {
		case <-ticker.C:
			// Wait for the server to start
			res, err := http.Get("http://" + apiAddr + "/blocks/0")
			if err != nil || res.StatusCode != http.StatusOK {
				continue
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				continue
			}

			var block blocks.JSONCollapsedBlock
			if err := json.Unmarshal(body, &block); err == nil {
				return nil
			}
		case <-timeout.C:
			return nil
		}
	}
}

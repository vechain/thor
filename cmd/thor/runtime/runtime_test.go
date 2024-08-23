package runtime

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/vechain/thor/v2/api/blocks"

	"github.com/stretchr/testify/assert"
)

func TestDefaultAction(t *testing.T) {
	dir := t.TempDir()
	defer os.RemoveAll(dir)
	apiAddr := newApiAddr(t)

	args := []string{os.Args[0], "--network", "main", "--data-dir", dir, "--api-addr", apiAddr}
	go func() {
		Start(args)
	}()

	assert.NoError(t, waitForServer(apiAddr))
}

func TestSoloAction(t *testing.T) {
	apiAddr := newApiAddr(t)

	args := []string{os.Args[0], "solo", "--api-addr", apiAddr, "solo"}
	go func() {
		Start(args)
	}()

	assert.NoError(t, waitForServer(apiAddr))
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
	timeout := time.NewTimer(10 * time.Second)

	for {
		select {
		case <-ticker.C:
			// Wait for the server to start
			res, err := http.Get("http://" + apiAddr + "/blocks/0")
			if err != nil && res.StatusCode != http.StatusOK {
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

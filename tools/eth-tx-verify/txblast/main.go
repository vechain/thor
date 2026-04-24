// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// txblast sends VeChain-native and Ethereum-style transactions against a solo
// node to verify the spec-1/2/3 implementation. See docs/superpowers/specs/
// 2026-04-24-eth-tx-verification-toolkit-design.md §3.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Solo dev account #0 — infinite VET + VTHO. See
// thor/genesis/devnet.go (or equivalent) for the canonical list.
const defaultSoloDevKey = "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36"

func main() {
	url := flag.String("url", "http://localhost:8669", "Solo node base URL")
	interval := flag.Duration("interval", 2*time.Second, "Batch interval")
	batch := flag.Int("batch", 1, "Multiplier per (type,path) cell (10 * batch = tx per tick)")
	key := flag.String("key", defaultSoloDevKey, "Hex private key (no 0x prefix)")
	dryRun := flag.Bool("dry-run", false, "Build & sign but do not submit")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("txblast starting url=%s interval=%s batch=%d dry=%t key=%s...\n",
		*url, *interval, *batch, *dryRun, (*key)[:8])

	// Task 2.6 will implement the actual batch loop here.
	// For now, just wait for Ctrl+C.
	<-ctx.Done()

	fmt.Println("txblast stopping")
}

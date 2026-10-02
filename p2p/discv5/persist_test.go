// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package discv5

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func listenLocal(t *testing.T, dbPath string) *Network {
	t.Helper()

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IP{127, 0, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	realaddr := conn.LocalAddr().(*net.UDPAddr)
	network, err := ListenUDP(key, conn, realaddr, dbPath, nil)
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return network
}

// TestSeedFromNodeDBAtStartup asserts that a node whose database holds peers
// from a previous run starts talking to them right away, without any bootnode
// configured.
func TestSeedFromNodeDBAtStartup(t *testing.T) {
	// the peer to be rediscovered, it records whoever verifies it
	seed := listenLocal(t, filepath.Join(t.TempDir(), "seed-nodes"))
	defer seed.Close()

	// leave the database as a previous run would have left it
	dbPath := filepath.Join(t.TempDir(), "nodes")
	db, err := newNodeDB(dbPath, Version, NodeID{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.updateNode(seed.Self()); err != nil {
		t.Fatal(err)
	}
	if err := db.updateLastPong(seed.Self().ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	db.close()

	restarted := listenLocal(t, dbPath)
	defer restarted.Close()

	restartedID := restarted.Self().ID
	deadline := time.Now().Add(20 * time.Second)
	for seed.db.node(restartedID) == nil {
		if time.Now().After(deadline) {
			t.Fatal("restarted node did not contact the peer persisted in its database")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestRefreshKeepsFallbackNodes asserts that the configured fallback nodes are
// contacted even when the database already holds seeds from a previous run.
func TestRefreshKeepsFallbackNodes(t *testing.T) {
	persisted := listenLocal(t, filepath.Join(t.TempDir(), "persisted-nodes"))
	defer persisted.Close()
	fallback := listenLocal(t, filepath.Join(t.TempDir(), "fallback-nodes"))
	defer fallback.Close()

	dbPath := filepath.Join(t.TempDir(), "nodes")
	db, err := newNodeDB(dbPath, Version, NodeID{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.updateNode(persisted.Self()); err != nil {
		t.Fatal(err)
	}
	if err := db.updateLastPong(persisted.Self().ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	db.close()

	restarted := listenLocal(t, dbPath)
	defer restarted.Close()
	if err := restarted.SetFallbackNodes([]*Node{fallback.Self()}); err != nil {
		t.Fatal(err)
	}

	restartedID := restarted.Self().ID
	deadline := time.Now().Add(20 * time.Second)
	for persisted.db.node(restartedID) == nil || fallback.db.node(restartedID) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("persisted seed reached: %t, fallback node reached: %t",
				persisted.db.node(restartedID) != nil, fallback.db.node(restartedID) != nil)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestPersistVerifiedPeers asserts that a node running with a persistent node
// database records the peers it verified, and hands them back as seeds after a
// restart.
func TestPersistVerifiedPeers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nodes")

	bootnode := listenLocal(t, dbPath)
	peer := listenLocal(t, "")
	defer peer.Close()

	if err := peer.SetFallbackNodes([]*Node{bootnode.Self()}); err != nil {
		t.Fatal(err)
	}

	peerID := peer.Self().ID
	deadline := time.Now().Add(20 * time.Second)
	for bootnode.db.node(peerID) == nil {
		if time.Now().After(deadline) {
			t.Fatal("bootnode did not persist the verified peer")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastPong := bootnode.db.lastPong(peerID); lastPong.IsZero() {
		t.Fatal("bootnode did not persist the last pong of the verified peer")
	}
	bootnode.Close()

	// restart on the same database, the peer must come back as a seed
	restarted := listenLocal(t, dbPath)
	defer restarted.Close()

	seeds := restarted.db.querySeeds(10, 24*time.Hour)
	for _, seed := range seeds {
		if seed.ID == peerID {
			return
		}
	}
	t.Fatalf("peer not returned as seed, got %d seeds", len(seeds))
}

// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// White-box regression tests for ticket-counter persistence and the
// :tickets cache bound.
package discv5

import (
	"net"
	"testing"
)

// countTicketKeys counts :tickets keys in the db.
func countTicketKeys(t *testing.T, db *nodeDB) int {
	t.Helper()
	it := db.lvl.NewIterator(nil, nil)
	defer it.Release()
	n := 0
	for it.Next() {
		if _, field := splitKey(it.Key()); field == nodeDBTopicRegTickets {
			n++
		}
	}
	return n
}

func mkNode(i int) *Node {
	var id NodeID
	id[0], id[1], id[2], id[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
	return NewNode(id, net.IP{1, 2, 3, 4}, 30303, 30303)
}

// Unbonded (unknown) nodes going through getTicket are not persisted;
// known nodes persist normally.
func TestTicketPersist_OnlyKnown(t *testing.T) {
	self := mkNode(0).ID
	db, err := newNodeDB("", Version, self)
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()
	tab := newTopicTable(db, mkNode(0))

	const N = 500
	for i := 1; i <= N; i++ {
		n := mkNode(i)
		n.state = unknown // not yet bonded
		tab.getTicket(n, nil)
	}
	if got := countTicketKeys(t, db); got != 0 {
		t.Fatalf("unknown nodes must not persist, :tickets=%d want 0", got)
	}

	known1 := mkNode(N + 1)
	known1.state = known
	tab.getTicket(known1, nil)
	if got := countTicketKeys(t, db); got != 1 {
		t.Fatalf("known node should persist normally, :tickets=%d want 1", got)
	}
}

// Negative control: a pong whose ReplyTok doesn't match pingEcho is rejected
// and does not advance the node to known. This is the round-trip check that
// ties reaching known to receiving our ping at the claimed address.
func TestMismatchedPongDoesNotReachKnown(t *testing.T) {
	net := &Network{} // checkPacket only reads pkt and n; db=nil so ensureExpirer in handle doesn't fire
	n := mkNode(1)
	n.state = verifywait                  // mid-state: we sent a ping and are waiting for the pong
	n.pingEcho = []byte{0xAA, 0xBB, 0xCC} // hash of our ping

	// A pong whose ReplyTok doesn't match pingEcho.
	mismatched := &ingressPacket{ev: pongPacket, data: &pong{ReplyTok: []byte{0xDE, 0xAD}}}
	err := net.handle(n, pongPacket, mismatched)
	if err == nil {
		t.Fatal("pong with mismatched ReplyTok must be rejected")
	}
	if n.state == known {
		t.Fatal("mismatched pong must not advance the node to known")
	}
	if n.state != verifywait {
		t.Fatalf("state should remain verifywait after rejection, got %v", n.state)
	}

	// Positive control: checkPacket accepts when ReplyTok == pingEcho (proves
	// the gate isn't always rejecting).
	valid := &ingressPacket{ev: pongPacket, data: &pong{ReplyTok: n.pingEcho}}
	if err := net.checkPacket(n, pongPacket, valid); err != nil {
		t.Fatalf("valid pong (ReplyTok==pingEcho) should not be rejected: %v", err)
	}
}

func hasTicket(db *nodeDB, id NodeID) bool {
	_, err := db.lvl.Get(makeKey(id, nodeDBTopicRegTickets), nil)
	return err == nil
}

// Writing more than maxTicketEntries distinct NodeIDs caps the total
// :tickets count and evicts the oldest entries.
func TestTicketsCap_Bounded(t *testing.T) {
	self := mkNode(0).ID
	db, err := newNodeDB("", Version, self)
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()

	total := maxTicketEntries + 100
	for i := 1; i <= total; i++ {
		db.updateTopicRegTickets(mkNode(i).ID, 1, 0)
	}

	if got := countTicketKeys(t, db); got != maxTicketEntries {
		t.Fatalf(":tickets should be capped at %d, got %d", maxTicketEntries, got)
	}
	if hasTicket(db, mkNode(1).ID) {
		t.Fatal("oldest NodeID#1 should have been evicted")
	}
	if !hasTicket(db, mkNode(total).ID) {
		t.Fatalf("newest NodeID#%d should still be present", total)
	}
}

// A persistent database that already holds more than maxTicketEntries
// :tickets keys (e.g. written by a pre-fix binary, or by a prior process
// before this cache cap existed) must be pruned back down to the cap on
// reopen, and the reopened LRU must know about the surviving on-disk entries
// rather than starting blind and only bounding writes made since reopen.
func TestTicketsCap_SeededOnReopen(t *testing.T) {
	dir := t.TempDir()
	self := mkNode(0).ID

	db, err := newNodeDB(dir, Version, self)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate legacy bloat by writing directly to leveldb, bypassing
	// db.tickets (and thus the LRU/eviction path) entirely.
	over := maxTicketEntries + 500
	blob := make([]byte, 8)
	for i := 1; i <= over; i++ {
		if err := db.lvl.Put(makeKey(mkNode(i).ID, nodeDBTopicRegTickets), blob, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := countTicketKeys(t, db); got != over {
		t.Fatalf("setup: expected %d raw :tickets entries, got %d", over, got)
	}
	db.close()

	reopened, err := newNodeDB(dir, Version, self)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()

	if got := countTicketKeys(t, reopened); got != maxTicketEntries {
		t.Fatalf("reopen should prune legacy surplus down to %d, got %d", maxTicketEntries, got)
	}

	// A further write should evict exactly one more entry rather than being
	// layered on top of the still-unpruned surplus, proving the reopened LRU
	// is tracking what's already on disk and not starting empty.
	reopened.updateTopicRegTickets(mkNode(over+1).ID, 1, 0)
	if got := countTicketKeys(t, reopened); got != maxTicketEntries {
		t.Fatalf("post-reopen write should keep :tickets capped at %d, got %d", maxTicketEntries, got)
	}
	if !hasTicket(reopened, mkNode(over+1).ID) {
		t.Fatal("newly written ticket should be present after reopen")
	}
}

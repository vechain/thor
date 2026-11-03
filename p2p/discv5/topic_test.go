// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package discv5

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
)

func TestTopicRadius(t *testing.T) {
	now := mclock.Now()
	topic := Topic("qwerty")
	rad := newTopicRadius(topic)
	targetRad := (^uint64(0)) / 100

	waitFn := func(addr common.Hash) time.Duration {
		prefix := binary.BigEndian.Uint64(addr[0:8])
		dist := prefix ^ rad.topicHashPrefix
		relDist := float64(dist) / float64(targetRad)
		relTime := (1 - relDist/2) * 2
		relTime = max(relTime, 0)
		return time.Duration(float64(targetWaitTime) * relTime)
	}

	bcnt := 0
	cnt := 0
	var sum float64
	for cnt < 100 {
		addr := rad.nextTarget(false).target
		wait := waitFn(addr)
		ticket := &ticket{
			topics:  []Topic{topic},
			regTime: []mclock.AbsTime{mclock.AbsTime(wait)},
			node:    &Node{nodeNetGuts: nodeNetGuts{sha: addr}},
		}
		rad.adjustWithTicket(now, addr, ticketRef{ticket, 0})
		if rad.radius != maxRadius {
			cnt++
			sum += float64(rad.radius)
		} else {
			bcnt++
			if bcnt > 500 {
				t.Errorf("Radius did not converge in 500 iterations")
			}
		}
	}
	avgRel := sum / float64(cnt) / float64(targetRad)
	if avgRel > 1.05 || avgRel < 0.95 {
		t.Errorf("Average/target ratio is too far from 1 (%v)", avgRel)
	}
}

func topicTableTestNode(i int) *Node {
	var id NodeID
	id[0], id[1], id[2], id[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
	port := uint16(20000 + i%40000)
	return NewNode(id, net.IPv4(127, 0, 0, 1), port, port)
}

func checkTopicTableInvariants(t *testing.T, tab *topicTable) {
	t.Helper()
	inHeap := make(map[Topic]bool)
	for _, item := range tab.requested {
		if item.index < 0 {
			t.Fatalf("heap item %v has stale index %d", item.topic, item.index)
		}
		inHeap[item.topic] = true
	}
	for topic := range tab.topics {
		if !inHeap[topic] {
			t.Fatalf("topic %v present in map but missing from heap", topic)
		}
	}
	var topicSum, nodeSum int
	for _, ti := range tab.topics {
		topicSum += len(ti.entries)
	}
	for node, n := range tab.nodes {
		nodeSum += len(n.entries)
		for topic, e := range n.entries {
			ti := tab.topics[topic]
			if ti == nil {
				t.Fatalf("ghost entry: node %x indexes topic %v missing from map", node.ID[:8], topic)
			}
			if ti.entries[e.fifoIdx] != e {
				t.Fatalf("node %x entry for %v not resolved by topic index", node.ID[:8], topic)
			}
			if e.node != node {
				t.Fatalf("entry for %v points at a different node", topic)
			}
		}
	}
	if topicSum != nodeSum || uint64(topicSum) != tab.globalEntries {
		t.Fatalf("entry counts diverge: topics=%d nodes=%d global=%d", topicSum, nodeSum, tab.globalEntries)
	}
}

// Registering into the single least-requested entry at the global cap used to
// insert through a topicInfo cached before the eviction, leaving a ghost entry
// that crashed deleteEntry on the same node's next registration or on expiry.
func TestAddEntrySameTopicEvictionKeepsIndexesConsistent(t *testing.T) {
	tab := newTopicTable(nil, topicTableTestNode(0))
	target := Topic("stale-target")
	tab.addEntry(topicTableTestNode(1), target)

	ti := tab.topics[target]
	ti.wcl.lastIncoming = mclock.Now() - mclock.AbsTime(wcTargetRegInterval+time.Second)
	if !ti.wcl.hasMinimumWaitPeriod() {
		t.Fatal("target topic wait did not decay to minimum")
	}
	for i := 2; i <= maxEntries; i++ {
		tab.addEntry(topicTableTestNode(i), Topic(fmt.Sprintf("fill-%05d", i)))
	}
	if tab.globalEntries != maxEntries || tab.requested[0].topic != target {
		t.Fatalf("bad setup: entries=%d root=%v", tab.globalEntries, tab.requested[0].topic)
	}

	replacement := topicTableTestNode(maxEntries + 1)
	tab.addEntry(replacement, target)
	checkTopicTableInvariants(t, tab)
	if ti = tab.topics[target]; ti == nil || len(ti.entries) != 1 {
		t.Fatalf("target topic not live with the replacement entry: %v", ti)
	}

	tab.addEntry(replacement, Topic("stale-second"))
	checkTopicTableInvariants(t, tab)
	if tab.globalEntries != maxEntries {
		t.Fatalf("cap not preserved: entries=%d", tab.globalEntries)
	}
}

// An empty topic whose adaptive wait had not decayed stays in the map and at
// the heap root; the next over-cap registration used to scan its empty entry
// map forever in getFifoTail, wedging the network loop goroutine.
func TestLeastRequestedSkipsEmptyTopics(t *testing.T) {
	tab := newTopicTable(nil, topicTableTestNode(0))
	trap := Topic("empty-trap")
	tab.addEntry(topicTableTestNode(1), trap)
	tab.addEntry(topicTableTestNode(2), trap)
	if tab.topics[trap].wcl.hasMinimumWaitPeriod() {
		t.Fatal("setup did not elevate trap wait period")
	}
	for i := 3; i <= maxEntries; i++ {
		tab.addEntry(topicTableTestNode(i), Topic(fmt.Sprintf("fill-%05d", i)))
	}
	tab.addEntry(topicTableTestNode(maxEntries+1), Topic("over-cap-1"))
	tab.addEntry(topicTableTestNode(maxEntries+2), Topic("over-cap-2"))
	if ti := tab.topics[trap]; ti == nil || len(ti.entries) != 0 {
		t.Fatalf("setup did not reach the retained-empty trap state: %d entries", len(ti.entries))
	}
	if tab.requested[0].topic != trap {
		t.Fatalf("empty trap is not heap root: %v", tab.requested[0].topic)
	}

	done := make(chan struct{})
	go func() {
		tab.addEntry(topicTableTestNode(maxEntries+3), Topic("trigger"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("addEntry did not return; leastRequested spun on the empty topic")
	}
	checkTopicTableInvariants(t, tab)
	if tab.globalEntries != maxEntries {
		t.Fatalf("cap not preserved: entries=%d", tab.globalEntries)
	}
}

func TestGetFifoTailEmpty(t *testing.T) {
	ti := &topicInfo{entries: make(map[uint64]*topicEntry)}
	if e := ti.getFifoTail(); e != nil {
		t.Fatalf("expected nil tail on empty map, got %v", e)
	}
}

// Defense in depth: an entry whose topic already vanished from the map must
// heal instead of crashing the process.
func TestDeleteEntryToleratesMissingTopic(t *testing.T) {
	tab := newTopicTable(nil, topicTableTestNode(0))
	node := topicTableTestNode(1)
	n := tab.getOrNewNode(node)
	ghost := &topicEntry{topic: Topic("ghost"), fifoIdx: 0, node: node}
	n.entries[ghost.topic] = ghost
	tab.globalEntries++

	tab.deleteEntry(ghost)
	if len(n.entries) != 0 || tab.globalEntries != 0 {
		t.Fatalf("entry not healed: nodeEntries=%d global=%d", len(n.entries), tab.globalEntries)
	}
}

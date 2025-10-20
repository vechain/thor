// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/p2psrv/tempdiscv5"
)

// Nodes slice of discovered nodes.
// It's rlp encode/decodable
type Nodes []*tempdiscv5.Node

// DecodeRLP implements rlp.Decoder.
func (ns *Nodes) DecodeRLP(s *rlp.Stream) error {
	_, err := s.List()
	if err != nil {
		return err
	}
	*ns = nil
	for {
		var n tempdiscv5.Node
		if err := s.Decode(&n); err != nil {
			if err != rlp.EOL {
				return err
			}
			return nil
		}
		*ns = append(*ns, tempdiscv5.NewNode(n.ID, n.IP, n.UDP, n.TCP, time.Now()))
	}
}

// thread-safe node map.
type nodeMap struct {
	m    map[tempdiscv5.NodeID]*tempdiscv5.Node
	lock sync.Mutex
}

func newNodeMap() *nodeMap {
	return &nodeMap{
		m: make(map[tempdiscv5.NodeID]*tempdiscv5.Node),
	}
}

func (nm *nodeMap) Add(node *tempdiscv5.Node) {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	nm.m[node.ID] = node
}

func (nm *nodeMap) Remove(id tempdiscv5.NodeID) *tempdiscv5.Node {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	if node, ok := nm.m[id]; ok {
		delete(nm.m, id)
		return node
	}
	return nil
}

func (nm *nodeMap) Contains(id tempdiscv5.NodeID) bool {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	return nm.m[id] != nil
}

func (nm *nodeMap) Len() int {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	return len(nm.m)
}

package p2psrv

import (
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Nodes slice of discovered nodes.
// It's rlp encode/decodable
type Nodes []*discover.Node

// DecodeRLP implements rlp.Decoder.
func (ns *Nodes) DecodeRLP(s *rlp.Stream) error {
	_, err := s.List()
	if err != nil {
		return err
	}
	*ns = nil
	for {
		var n discover.Node
		if err := s.Decode(&n); err != nil {
			if err != rlp.EOL {
				return err
			}
			return nil
		}
		*ns = append(*ns, discover.NewNode(n.ID, n.IP, n.UDP, n.TCP))
	}
}

// To keep a set of nodes with length limit.
// If length exceeds max, a randomly selected node will be popped.
type nodePool struct {
	m      map[discover.NodeID]*discover.Node
	s      []*discover.Node
	maxLen int
	lock   sync.Mutex
}

func newNodePool(maxLen int) *nodePool {
	return &nodePool{
		m:      make(map[discover.NodeID]*discover.Node),
		maxLen: maxLen,
	}
}

func (np *nodePool) Add(node *discover.Node) bool {
	np.lock.Lock()
	defer np.lock.Unlock()
	if _, found := np.m[node.ID]; found {
		return false
	}
	if len(np.s) >= np.maxLen {
		np.randPop()
	}
	np.m[node.ID] = node
	np.s = append(np.s, node)
	return true
}

func (np *nodePool) Len() int {
	np.lock.Lock()
	defer np.lock.Unlock()
	return len(np.s)
}

func (np *nodePool) RandGet() *discover.Node {
	np.lock.Lock()
	defer np.lock.Unlock()
	if len(np.s) == 0 {
		return nil
	}
	return np.s[rand.Intn(len(np.s))]
}

func (np *nodePool) randPop() *discover.Node {
	n := len(np.s)
	if n == 0 {
		return nil
	}
	i := rand.Intn(n)
	// move last elem to i
	node := np.s[i]
	np.s[i] = np.s[n-1]
	np.s = np.s[:n-1]

	delete(np.m, node.ID)
	return node
}

// thread-safe node map.
type nodeMap struct {
	m    map[discover.NodeID]*discover.Node
	lock sync.Mutex
}

func newNodeMap() *nodeMap {
	return &nodeMap{
		m: make(map[discover.NodeID]*discover.Node),
	}
}

func (nm *nodeMap) Add(node *discover.Node) {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	nm.m[node.ID] = node
}

func (nm *nodeMap) Remove(id discover.NodeID) *discover.Node {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	if node, ok := nm.m[id]; ok {
		delete(nm.m, id)
		return node
	}
	return nil
}

func (nm *nodeMap) Contains(id discover.NodeID) bool {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	return nm.m[id] != nil
}

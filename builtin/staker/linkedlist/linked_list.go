// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package linkedlist

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

type Node struct {
	Next *thor.Address `rlp:"nil"`
	Prev *thor.Address `rlp:"nil"`
}

func (n *Node) DecodeSlots(slots []thor.Bytes32) error {
	if len(slots) != 2 {
		return errors.New("invalid number of slots for Node")
	}

	if !slots[0].IsZero() {
		next := thor.BytesToAddress(slots[0].Bytes())
		n.Next = &next
	}

	if !slots[1].IsZero() {
		prev := thor.BytesToAddress(slots[1].Bytes())
		n.Prev = &prev
	}

	return nil
}

func (n *Node) EncodeSlots() []thor.Bytes32 {
	slots := make([]thor.Bytes32, 2)

	if n.Next != nil {
		slots[0] = thor.BytesToBytes32(n.Next.Bytes())
	}

	if n.Prev != nil {
		slots[1] = thor.BytesToBytes32(n.Prev.Bytes())
	}

	return slots
}

func (n *Node) UsedSlots() int {
	return 2 // Each node uses two slots: one for Next and one for Prev
}

var _ solidity.ComplexValue[Node] = (*Node)(nil)

type LinkedList struct {
	mapping *solidity.ComplexMapping[thor.Address, *Node]
	head  *solidity.Address
	tail  *solidity.Address
	count *solidity.Uint256
}

// NewLinkedList creates a new linked list with persistent storage mappings for staker management
func NewLinkedList(
	sctx *solidity.Context,
	firstSlot thor.Bytes32,
) *LinkedList {
	headSlot := solidity.IncrementSlot(firstSlot[:], 1)
	tailSlot := solidity.IncrementSlot(firstSlot[:], 2)
	countPos := solidity.IncrementSlot(firstSlot[:], 3)
	return &LinkedList{
		mapping: solidity.NewComplexMapping[thor.Address, *Node](sctx, firstSlot),
		head:  solidity.NewAddress(sctx, headSlot),
		tail:  solidity.NewAddress(sctx, tailSlot),
		count: solidity.NewUint256(sctx, countPos),
	}
}

// Iter traverses the list in FIFO order, calling callback for each address until completion or error
func (l *LinkedList) Iter(callbacks ...func(thor.Address) error) error {
	ptr, err := l.head.Get()
	if err != nil {
		return err
	}

	for !ptr.IsZero() {
		node, err := l.mapping.Get(ptr)
		if err != nil {
			return errors.Wrapf(err, "failed to get node for address %s", ptr)
		}
		for _, callback := range callbacks {
			if err := callback(ptr); err != nil {
				return err
			}
		}
		if node.Next == nil {
			break
		}
		ptr = *node.Next
	}

	return nil
}

// Add appends an address to the end of the list, maintaining FIFO order for staker processing
func (l *LinkedList) Add(address thor.Address) error {
	oldTail, err := l.tail.Get()
	if err != nil {
		return err
	}

	if oldTail.IsZero() {
		// the list is currently empty, set this entry to head & tail
		l.head.Set(&address, false)
		l.tail.Set(&address, false)
		return l.count.Add(big.NewInt(1))
	}

	prev, err := l.mapping.Get(oldTail)
	if err != nil {
		return errors.Wrapf(err, "failed to get previous tail node for address %s", oldTail)
	}

	// Set new address as next of old tail
	prev.Next = &address
	l.mapping.Set(oldTail, prev)

	// Create new node for the new address
	newNode := Node{Prev: &oldTail}
	l.mapping.Set(address, &newNode)
	// Update tail pointer to the new address
	l.tail.Set(&address, false)

	if err := l.count.Add(big.NewInt(1)); err != nil {
		return err
	}

	return err
}

// Remove extracts an address from anywhere in the list, reconnecting adjacent nodes and clearing removed node's pointers
func (l *LinkedList) Remove(address thor.Address) error {
	if address.IsZero() {
		return nil
	}

	node, err := l.mapping.Get(address)
	if err != nil {
		return errors.Wrapf(err, "failed to get node for address %s", address)
	}

	if node.Next == nil && node.Prev == nil {
		head, err := l.isHead(address)
		if err != nil {
			return errors.Wrapf(err, "failed to check if address %s is head", address)
		}
		if !head {
			return errors.New("address not found in the list")
		}
		l.head.Set(&thor.Address{}, false) // If it's the only node, reset head
		l.tail.Set(&thor.Address{}, false) // and tail
		return l.count.Sub(big.NewInt(1))  // Decrement count
	}
	if node.Prev != nil {
		prevNode, err := l.mapping.Get(*node.Prev)
		if err != nil {
			return errors.Wrapf(err, "failed to get previous node for address %s", *node.Prev)
		}
		prevNode.Next = node.Next // Link previous node to next
		l.mapping.Set(*node.Prev, prevNode)
	} else {
		// If no previous node, this is the head
		l.head.Set(node.Next, false)
	}

	if node.Next != nil {
		nextNode, err := l.mapping.Get(*node.Next)
		if err != nil {
			return errors.Wrapf(err, "failed to get next node for address %s", *node.Next)
		}
		nextNode.Prev = node.Prev // Link next node to previous
		l.mapping.Set(*node.Next, nextNode)
	} else {
		// If no next node, this is the tail
		l.tail.Set(node.Prev, false)
	}

	if err := l.count.Sub(big.NewInt(1)); err != nil {
		return err
	}

	return err
}

// Pop removes and returns the oldest entry (head) for FIFO processing order
func (l *LinkedList) Pop() (thor.Address, error) {
	head, err := l.head.Get()
	if err != nil {
		return thor.Address{}, errors.New("no head present")
	}

	if head.IsZero() {
		return thor.Address{}, errors.New("list is empty")
	}

	// otherwise, remove and return
	if err := l.Remove(head); err != nil {
		return thor.Address{}, err
	}
	return head, nil
}

// Peek returns the next address to be processed without removing it from the queue
func (l *LinkedList) Peek() (thor.Address, error) {
	return l.head.Get()
}

// Len returns the current number of addresses in the staker queue
func (l *LinkedList) Len() (*big.Int, error) {
	//count := big.NewInt(0)
	//current, err := l.head.Get()
	//if err != nil {
	//	return count, errors.Wrap(err, "failed to get head address")
	//}
	//if current.IsZero() {
	//	return count, nil // If the list is empty, return zero count
	//}
	//
	//for {
	//	node, err := l.mapping.Get(current)
	//	if err != nil {
	//		return count, errors.Wrapf(err, "failed to get node for address %s", current)
	//	}
	//	count.Add(count, big.NewInt(1))
	//	if node.Next == nil {
	//		break // Reached the end of the list
	//	}
	//	current = *node.Next
	//}
	//
	//return count, nil
	return l.count.Get()
}

// Next returns the successor address in the list, or zero address if at the end
func (l *LinkedList) Next(address thor.Address) (thor.Address, error) {
	node, err := l.mapping.Get(address)
	if err != nil {
		return thor.Address{}, errors.Wrapf(err, "failed to get node for address %s", address)
	}
	if node.Next == nil {
		return thor.Address{}, nil // No next address, return zero address
	}
	return *node.Next, nil
}

// isHead checks if the given address is the head of the list
func (l *LinkedList) isHead(address thor.Address) (bool, error) {
	head, err := l.head.Get()
	if err != nil {
		return false, err
	}
	return head == address, nil
}

// Head returns the oldest address in the queue (next to be processed)
func (l *LinkedList) Head() (thor.Address, error) {
	return l.head.Get()
}

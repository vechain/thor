package testnode

import (
	"fmt"

	"github.com/vechain/thor/v2/test/testchain"
)

// NodeBuilder implements the builder pattern for creating a test node instance
type NodeBuilder struct {
	chain *testchain.Chain
}

// NewNodeBuilder creates a new NodeBuilder with default configuration
func NewNodeBuilder() *NodeBuilder {
	return &NodeBuilder{}
}

// WithChain sets the chain for the node.
// If not set, a default chain will be created during Build().
func (b *NodeBuilder) WithChain(chain *testchain.Chain) *NodeBuilder {
	if chain == nil {
		panic("chain cannot be nil")
	}
	b.chain = chain
	return b
}

// Build creates a new Node with the current configuration.
// Returns an error if the chain creation fails.
// If no chain was explicitly set, a default chain will be created
// with the default fork configuration.
func (b *NodeBuilder) Build() (Node, error) {
	var err error
	// Create the chain first
	chain := b.chain
	if chain == nil {
		chain, err = testchain.NewWithFork(&testchain.DefaultForkConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create default chain: %w", err)
		}
	}

	// Create the node
	return &node{
		chain: chain,
	}, nil
}

// Convenience constructors

// NewDefaultNode creates a new node with default configuration
func NewDefaultNode() (Node, error) {
	return NewNodeBuilder().Build()
}

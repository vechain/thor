package genesis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/genesis"
)

// TestNewMainnet tests the NewMainnet function for creating the mainnet genesis block
func TestNewMainnet(t *testing.T) {
	genesisBlock := genesis.NewMainnet()

	// Check if the returned genesis block is not nil
	assert.NotNil(t, genesisBlock, "NewMainnet should return a non-nil Genesis object")

	// Verify the basic settings of the Genesis object
	assert.Equal(t, "mainnet", genesisBlock.Name(), "Genesis name should be 'mainnet'")
	assert.NotEqual(t, uint64(0), genesisBlock.ID(), "Genesis ID should not be zero")

	// Additional checks can include verifying the launch time, initial authority nodes, token supply, etc.
	// These will depend on the specifics of your implementation
}

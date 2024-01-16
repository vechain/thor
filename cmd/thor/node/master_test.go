package node

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestAddress(t *testing.T) {
	// Generate a new private key
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create a new Master instance
	master := &Master{
		PrivateKey: privateKey,
	}

	// Compute the expected address
	expectedAddress := thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey))

	// Use the Address method
	resultAddress := master.Address()

	// Assert that the computed address is correct
	assert.Equal(t, expectedAddress, resultAddress, "The computed address does not match the expected address")
}

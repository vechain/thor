package e2e

import (
	"github.com/vechain/thor/v2/tests/e2e/client"
	"github.com/vechain/thor/v2/tests/e2e/network"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBlock(t *testing.T) {
	network.StartCompose(t)

	block, err := client.Default.GetCompressedBlock(1)

	assert.NoError(t, err, "GetCompressedBlock()")
	assert.Greater(t, block.Number, uint32(0), "GetCompressedBlock()")
}

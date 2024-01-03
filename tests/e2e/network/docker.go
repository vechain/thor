package network

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	"testing"
)

type Network struct {
	stack *compose.ComposeStack
}

// StartCompose starts the docker-compose network and destroys it after the test
func StartCompose(t *testing.T) {
	dc, err := compose.NewDockerCompose("network/docker-compose.yaml")
	assert.NoError(t, err, "NewDockerComposeAPI()")

	t.Cleanup(func() {
		assert.NoError(t, dc.Down(context.Background(), compose.RemoveOrphans(true), compose.RemoveImagesLocal), "compose.Down()")
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	assert.NoError(t, dc.Up(ctx, compose.Wait(true)), "compose.Up()")
}

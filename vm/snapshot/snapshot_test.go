package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnapshot_Fullback(t *testing.T) {
	assert := assert.New(t)

	snapshotManager := New()
	snapshotManager.AddSnapshot(5)
	snapshotManager.AddSnapshot(6)

	assert.Equal(snapshotManager.Fullback(), 5, "获取到的应该是倒数第二个插入的元素.")
}

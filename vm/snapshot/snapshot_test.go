package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testVersion struct {
	num int
}

func (tv *testVersion) DeepCopy() interface{} {
	return &testVersion{
		num: tv.num,
	}
}

func TestSnapshot(t *testing.T) {
	assert := assert.New(t)

	snapshotManager := New()

	tv := &testVersion{10}
	snapshotManager.Snapshot(tv) // ver == 0

	tv.num = 20
	snapshotManager.Snapshot(tv) // ver == 1

	tv.num = 30
	ver := snapshotManager.Snapshot(tv) // ver == 2
	assert.Equal(ver, 2, "版本号应该等于 2.")

	tv.num = 40

	tv = snapshotManager.RevertToSnapshot(1).(*testVersion)
	assert.Equal(tv.num, 20, "外部对象应该回退到版本: num == 20.")

	tv = snapshotManager.RevertToSnapshot(2).(*testVersion)
	assert.Equal(tv.num, 20, "当前版本号大于当前版本, 直接返回当前版本: num == 20.")
}

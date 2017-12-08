package snapshot

type version interface {
	DeepCopy() interface{}
}

// Snapshot is the version control.
// Implements evm.ISnapshoter.
type Snapshot struct {
	snapshots []interface{}
}

// New return a new Snapshot point.
func New() *Snapshot {
	return &Snapshot{
		snapshots: make([]interface{}, 0),
	}
}

// Snapshot add a snapshot for snapshot.
// Starting from 0.
func (sn *Snapshot) Snapshot(snapshot version) int {
	sn.snapshots = append(sn.snapshots, snapshot.DeepCopy())
	return len(sn.snapshots) - 1
}

// RevertToSnapshot delete the last snapshot.
func (sn *Snapshot) RevertToSnapshot(ver int) interface{} {
	lenth := len(sn.snapshots)
	if ver >= lenth {
		return sn.snapshots[lenth-1]
	}
	sn.snapshots = sn.snapshots[:ver+1]
	return sn.snapshots[ver]
}

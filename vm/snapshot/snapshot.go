package snapshot

// Snapshot is the version control for account.Manager.
// Implements vm.ISnapshoter.
type Snapshot struct {
	versions []interface{}
}

// New return a new Snapshot point.
func New() *Snapshot {
	return &Snapshot{
		versions: make([]interface{}, 0),
	}
}

// AddSnapshot add a snapshot for snapshot.
func (sn *Snapshot) AddSnapshot(snapshot interface{}) {
	sn.versions = append(sn.versions, snapshot)
}

// GetLastSnapshot go to the last added snapshot.
func (sn *Snapshot) GetLastSnapshot() interface{} {
	return sn.versions[len(sn.versions)-1]
}

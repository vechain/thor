package snapshot

// Snapshot is the version control.
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

// Fullback delete the last snapshot.
func (sn *Snapshot) Fullback() interface{} {
	sn.versions = sn.versions[:len(sn.versions)-1]
	return sn.versions[len(sn.versions)-1]
}

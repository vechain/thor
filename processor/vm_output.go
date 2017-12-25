package processor

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/vm"
)

// StateWriter provides state writing methods.
type StateWriter interface {
	Update(acc.Address, *acc.Account) error
	Delete(acc.Address) error
}

// StorageWriter provides storage writing methods.
type StorageWriter interface {
	Update(cry.Hash, cry.Hash, cry.Hash) error
	Root(cry.Hash) (cry.Hash, error)
}

// KVPutter provides kv writing methods.
type KVPutter interface {
	Put([]byte, []byte) error
}

// VMOutput alias of vm.Output
type VMOutput vm.Output

// ApplyState apply account state changes.
func (o *VMOutput) ApplyState(state StateWriter, storage StorageWriter, kv KVPutter) error {
	for _, da := range o.DirtiedAccounts {
		if da.Suicided || da.Data.IsEmpty() {
			if err := state.Delete(da.Address); err != nil {
				return err
			}
		} else {
			accCopy := *da.Data
			// update storage
			for k, v := range da.DirtyStorage {
				if err := storage.Update(accCopy.StorageRoot, k, v); err != nil {
					return err
				}
			}

			if len(da.DirtyCode) > 0 {
				// writing code
				if err := kv.Put(accCopy.CodeHash[:], da.DirtyCode); err != nil {
					return err
				}
			}

			// update account itself
			accCopy.StorageRoot, _ = storage.Root(accCopy.StorageRoot)
			if err := state.Update(da.Address, &accCopy); err != nil {
				return err
			}
		}
	}
	return nil
}

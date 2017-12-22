package processor

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/vm"
)

// StateWriter provides state writing methods.
type StateWriter interface {
	UpdateAccount(acc.Address, *acc.Account) error
	Delete(key []byte) error
}

// StorageWriter provides storage writing methods.
type StorageWriter interface {
	UpdateStorage(root cry.Hash, key cry.Hash, value cry.Hash) error
	Hash(root cry.Hash) cry.Hash
}

// KVPutter provides kv writing methods.
type KVPutter interface {
	Put(key, value []byte) error
}

// VMOutput alias of vm.Output
type VMOutput vm.Output

// ApplyState apply account state changes.
func (o *VMOutput) ApplyState(state StateWriter, storage StorageWriter, kv KVPutter) error {
	for _, da := range o.DirtiedAccounts {
		if da.Suicided || da.Data.IsEmpty() {
			if err := state.Delete(da.Address[:]); err != nil {
				return err
			}
		} else {
			accCopy := *da.Data
			// update storage
			for k, v := range da.DirtyStorage {
				if err := storage.UpdateStorage(accCopy.StorageRoot, k, v); err != nil {
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
			accCopy.StorageRoot = storage.Hash(accCopy.StorageRoot)
			if err := state.UpdateAccount(da.Address, &accCopy); err != nil {
				return err
			}
		}
	}
	return nil
}

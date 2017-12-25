package processor

import (
	"github.com/vechain/thor/vm"
)

// VMOutput alias of vm.Output
type VMOutput vm.Output

// ApplyState apply account state changes.
func (o *VMOutput) ApplyState(state Stater) error {
	for _, da := range o.DirtiedAccounts {
		if da.Suicided {
			state.DeleteAccount(da.Address)
		} else {
			// update storage
			for k, v := range da.DirtyStorage {
				state.SetStorage(da.Address, k, v)
				if err := state.Error(); err != nil {
					return err
				}
			}

			// update code
			if len(da.DirtyCode) > 0 {
				state.SetCode(da.Address, da.DirtyCode)
			}

			// update balance
			state.SetBalance(da.Address, da.Balance)
		}
	}

	return state.Error()
}

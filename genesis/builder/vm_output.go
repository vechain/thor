package builder

import (
	"github.com/vechain/thor/vm"
)

// VMOutput alias of vm.Output
type VMOutput vm.Output

// ApplyState apply account state changes.
func (o *VMOutput) ApplyState(state State) error {
	for addr, account := range o.Accounts {
		if account.Suicided {
			state.Delete(addr)
			if err := state.Error(); err != nil {
				return err
			}
		} else {
			// update code
			if len(account.Code) > 0 {
				state.SetCode(addr, account.Code)
				if err := state.Error(); err != nil {
					return err
				}
			}

			// update balance
			state.SetBalance(addr, account.Balance)
			if err := state.Error(); err != nil {
				return err
			}
		}
	}

	for key, value := range o.Storages {
		state.SetStorage(key.Addr, key.Key, value)
		if err := state.Error(); err != nil {
			return err
		}
	}

	return nil
}

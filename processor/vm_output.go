package processor

import (
	"github.com/vechain/thor/vm"
)

// VMOutput alias of vm.Output
type VMOutput vm.Output

// ApplyState apply account state changes.
func (o *VMOutput) ApplyState(state Stater) error {
	for addr, account := range o.Accounts {
		if account.Suicided() {
			state.DeleteAccount(addr)
			if err := state.Error(); err != nil {
				return err
			}
		} else {
			// update code
			code := account.Code()
			if len(code) > 0 {
				state.SetCode(addr, code)
				if err := state.Error(); err != nil {
					return err
				}
			}

			// update balance
			state.SetBalance(addr, account.Balance())
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

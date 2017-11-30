package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/vecore/acc"
)

type journalEntry interface {
	undo(*Manager)
}

type journal []journalEntry

type (
	// Changes to the account trie.
	createObjectChange struct {
		account *acc.Address
	}
	resetObjectChange struct {
		prev *Account
	}
	suicideChange struct {
		account     *acc.Address
		prev        bool // whether account had already suicided
		prevbalance *big.Int
	}

	// Changes to individual accounts.
	balanceChange struct {
		account *acc.Address
		prev    *big.Int
	}
	nonceChange struct {
		account *acc.Address
		prev    uint64
	}
	storageChange struct {
		account       *acc.Address
		key, prevalue common.Hash
	}
	codeChange struct {
		account            *acc.Address
		prevcode, prevhash []byte
	}

	// Changes to other state values.
	refundChange struct {
		prev *big.Int
	}
	addLogChange struct {
		txhash common.Hash
	}
	addPreimageChange struct {
		hash common.Hash
	}
	touchChange struct {
		account   *acc.Address
		prev      bool
		prevDirty bool
	}
)

// func (ch createObjectChange) undo(s *Manager) {
// 	delete(s.accounts, *ch.account)
// 	delete(s.accountsDirty, *ch.account)
// }

// func (ch resetObjectChange) undo(s *Manager) {
// 	s.setStateObject(ch.prev)
// }

// func (ch suicideChange) undo(s *Manager) {
// 	obj := s.getStateObject(*ch.account)
// 	if obj != nil {
// 		obj.suicided = ch.prev
// 		obj.setBalance(ch.prevbalance)
// 	}
// }

// var ripemd = common.HexToacc.Address("0000000000000000000000000000000000000003")

// func (ch touchChange) undo(s *Manager) {
// 	if !ch.prev && *ch.account != ripemd {
// 		s.getStateObject(*ch.account).touched = ch.prev
// 		if !ch.prevDirty {
// 			delete(s.stateObjectsDirty, *ch.account)
// 		}
// 	}
// }

func (ch balanceChange) undo(s *Manager) {
	s.SetBalance(*ch.account, ch.prev)
}

// func (ch nonceChange) undo(s *Manager) {
// 	s.getStateObject(*ch.account).setNonce(ch.prev)
// }

// func (ch codeChange) undo(s *Manager) {
// 	s.getStateObject(*ch.account).setCode(common.BytesToHash(ch.prevhash), ch.prevcode)
// }

// func (ch storageChange) undo(s *Manager) {
// 	s.getStateObject(*ch.account).setState(ch.key, ch.prevalue)
// }

func (ch refundChange) undo(s *Manager) {
	s.refund = ch.prev
}

// func (ch addLogChange) undo(s *Manager) {
// 	logs := s.logs[ch.txhash]
// 	if len(logs) == 1 {
// 		delete(s.logs, ch.txhash)
// 	} else {
// 		s.logs[ch.txhash] = logs[:len(logs)-1]
// 	}
// 	s.logSize--
// }

// func (ch addPreimageChange) undo(s *Manager) {
// 	delete(s.preimages, ch.hash)
// }

// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package common

import (
	"fmt"
)

const (
	FinalizedRevision = "finalized"
)

var (
	ErrNotFound      = fmt.Errorf("not found")
	ErrNot200Status  = fmt.Errorf("not 200 status code")
	ErrUnexpectedMsg = fmt.Errorf("unexpected message format")
)

// EventWrapper is used to return errors from the websocket alongside the data
type EventWrapper[T any] struct {
	Data  T
	Error error
}

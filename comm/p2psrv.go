package comm

import "github.com/vechain/thor/p2psrv"

type p2pServer interface {
	SessionSet() *p2psrv.SessionSet
}

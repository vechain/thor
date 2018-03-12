package comm

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/p2psrv"
)

type Communicator struct {
}

func New() *Communicator {
	return &Communicator{}
}

func (c *Communicator) Protocols() []*p2psrv.Protocol {

	return []*p2psrv.Protocol{
		&p2psrv.Protocol{
			Name:          proto.Name,
			Version:       proto.Version,
			Length:        proto.Length,
			HandleRequest: c.handleRequest,
		},
	}
}

func (c *Communicator) handleRequest(session *p2psrv.Session, msg *p2p.Msg) (resp interface{}) {
	return nil
}

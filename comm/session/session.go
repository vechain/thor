package session

import (
	"errors"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/co"
)

// HandleRequest handles incoming request message, acts like a server.
// The returned resp can be nil if don't want to response to incoming request.
type HandleRequest func(session *Session, msg *p2p.Msg) (resp interface{})

// Session p2p session which conforms request-response manner.
type Session struct {
	peer       *p2p.Peer
	nextReqSeq uint32
	localReqCh chan interface{}
	closed     chan interface{}
}

// New create a new session.
func New(peer *p2p.Peer) *Session {
	return &Session{
		peer,
		0,
		make(chan interface{}),
		make(chan interface{}),
	}
}

// Disconnect initiatively disconnect with remote peer.
func (s *Session) Disconnect() {
	s.peer.Disconnect(p2p.DiscSelf)
}

// Serve handles p2p message.
func (s *Session) Serve(ws p2p.MsgReadWriter, handleRequest HandleRequest) error {
	defer func() { close(s.closed) }()

	var runner co.Runner
	defer runner.Wait()

	type remoteResponse struct {
		sequence uint32
		msg      *p2p.Msg
	}
	remoteRespCh := make(chan *remoteResponse)
	defer close(remoteRespCh)

	runner.Go(func() {
		// handle local request and remote response
		pendingReqs := make(map[uint32]*request)
		for {
			select {
			case v := <-s.localReqCh:
				if req, ok := v.(*request); ok {
					if _, loaded := pendingReqs[req.sequence]; loaded {
						req.respCh <- &response{err: errors.New("duplicated sequence number")}
						continue
					}

					pendingReqs[req.sequence] = req
					if err := p2p.Send(ws, req.msgCode, []interface{}{req.sequence, false, req.payload}); err != nil {
						delete(pendingReqs, req.sequence)
						req.respCh <- &response{err: err}
						continue
					}
				} else if seq, ok := v.(uint32); ok {
					delete(pendingReqs, seq)
				} else {
					panic("unexpected")
				}
			case resp := <-remoteRespCh:
				if resp == nil {
					// session should be ended
					return
				}

				req, loaded := pendingReqs[resp.sequence]
				if !loaded {
					resp.msg.Discard()
					continue
				}

				if resp.msg.Code != req.msgCode {
					resp.msg.Discard()
					continue
				}
				delete(pendingReqs, resp.sequence)
				req.respCh <- &response{msg: resp.msg}
			}
		}

	})

	// read msg loop
	for {
		msg, err := ws.ReadMsg()
		if err != nil {
			return err
		}
		var (
			sequence   uint32
			isResponse bool
		)

		// parse firt two elements, which are sequence and isResponse
		stream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := stream.List(); err != nil {
			return err
		}
		if err := stream.Decode(&sequence); err != nil {
			return err
		}
		if err := stream.Decode(&isResponse); err != nil {
			return err
		}

		if isResponse {
			remoteRespCh <- &remoteResponse{sequence, &msg}
		} else {
			resp := handleRequest(s, &msg)
			msg.Discard()
			if resp != nil {
				if err := p2p.Send(ws, msg.Code, []interface{}{sequence, true, resp}); err != nil {
					return err
				}
			}
		}
	}
}

package session

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
)

// Request help to make request to Session, and handle corresponded response.
type Request struct {
	msgCode uint64
	payload interface{}
}

// NewRequest create a new request.
func NewRequest(session *Session, msgCode uint64, payload interface{}) *Request {
	return &Request{msgCode, payload}
}

// Do perform the request at session.
func (r *Request) Do(ctx context.Context, session *Session, respData interface{}) error {
	if session.closed == nil {
		return errors.New("session not started")
	}

	seq := atomic.AddUint32(&session.nextReqSeq, 1)
	req := &request{
		make(chan *response, 1),
		r.msgCode,
		seq,
		r.payload,
	}

	defer func() {
		select {
		case session.localReqCh <- seq:
		case <-session.closed:
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	select {
	case session.localReqCh <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-session.closed:
		return errors.New("session closed")
	}

	select {
	case resp := <-req.respCh:
		if resp.err != nil {
			return resp.err
		}
		defer resp.msg.Discard()
		return resp.msg.Decode(respData)
	case <-ctx.Done():
		return ctx.Err()
	case <-session.closed:
		return errors.New("session closed")
	}
}

type request struct {
	respCh   chan *response
	msgCode  uint64
	sequence uint32
	payload  interface{}
}

type response struct {
	err error
	msg *p2p.Msg
}

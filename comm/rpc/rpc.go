package rpc

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
)

func init() {
	// required when generate call id
	rand.Seed(time.Now().UnixNano())
}

const (
	rpcDefaultTimeout = time.Second * 10
)

var (
	errPeerDisconnected = errors.New("peer disconnected")
	errMsgTooLarge      = errors.New("msg too large")
	log                 = log15.New("pkg", "rpc")
)

type RPC struct {
	rw       p2p.MsgReadWriter
	doneCh   chan struct{}
	pendings map[uint32]*resultListener
	lock     sync.Mutex
	logger   log15.Logger
}

func New(rw p2p.MsgReadWriter, logCtx []interface{}) *RPC {
	return &RPC{
		rw:       rw,
		doneCh:   make(chan struct{}),
		pendings: make(map[uint32]*resultListener),
		logger:   log.New(logCtx...),
	}
}
func (r *RPC) Done() <-chan struct{} {
	return r.doneCh
}

func (r *RPC) Serve(handleFunc HandleFunc, maxMsgSize uint32) error {
	defer func() { close(r.doneCh) }()

	processMsg := func() error {
		msg, err := r.rw.ReadMsg()
		if err != nil {
			return err
		}
		// ensure msg.Payload consumed
		defer msg.Discard()

		if msg.Size > maxMsgSize {
			return errMsgTooLarge
		}
		// parse first two elements, which are callID and isResult
		stream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := stream.List(); err != nil {
			r.logger.Debug("failed to decode msg", "err", err)
			return err
		}
		var (
			callID   uint32
			isResult bool
		)
		if err := stream.Decode(&callID); err != nil {
			r.logger.Debug("failed to decode msg call id", "err", err)
			return err
		}
		if err := stream.Decode(&isResult); err != nil {
			r.logger.Debug("failed to decode msg dir flag", "err", err)
			return err
		}

		if isResult {
			if err := r.handleResult(callID, &msg); err != nil {
				r.logger.Debug("handle result", "msg", msg.Code, "callid", callID, "err", err)
			}
		} else {
			if err := handleFunc(&msg, func(result interface{}) {
				p2p.Send(r.rw, msg.Code, &msgData{callID, true, result})
			}); err != nil {
				r.logger.Debug("handle call", "msg", msg.Code, "callid", callID, "err", err)
			}
		}
		return nil
	}

	for {
		if err := processMsg(); err != nil {
			return err
		}
	}
}

func (r *RPC) handleResult(callID uint32, msg *p2p.Msg) error {
	r.lock.Lock()
	listener, ok := r.pendings[callID]
	if ok {
		delete(r.pendings, callID)
	}
	r.lock.Unlock()

	if !ok {
		return errors.New("unexpected call result")
	}

	if listener.msgCode != msg.Code {
		return errors.New("msg code mismatch")
	}

	if err := listener.onResult(msg, nil); err != nil {
		return err
	}
	return nil
}

func (r *RPC) prepareCall(msgCode uint64, onResult func(*p2p.Msg, error) error) uint32 {
	r.lock.Lock()
	defer r.lock.Unlock()
	for {
		id := rand.Uint32()
		if _, ok := r.pendings[id]; !ok {
			r.pendings[id] = &resultListener{
				msgCode,
				onResult,
			}
			return id
		}
	}
}
func (r *RPC) finalizeCall(id uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()
	delete(r.pendings, id)
}

func (r *RPC) Call(ctx context.Context, msgCode uint64, arg interface{}, result interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, rpcDefaultTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	id := r.prepareCall(msgCode, func(msg *p2p.Msg, err error) error {
		defer func() {
			errCh <- err
		}()
		if err != nil {
			return err
		}
		if err = msg.Decode(result); err != nil {
			return err
		}
		return nil
	})
	defer r.finalizeCall(id)

	if err := p2p.Send(r.rw, msgCode, &msgData{id, false, arg}); err != nil {
		return err
	}

	select {
	case <-r.doneCh:
		return errPeerDisconnected
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

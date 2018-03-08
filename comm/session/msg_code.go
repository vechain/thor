package session

import (
	"fmt"
)

type MsgCode uint64

func NewMsgCode(code uint8, seq uint32, isResponse bool) MsgCode {
	var flag MsgCode
	if isResponse {
		flag = 1
	}
	return (flag << 40) | MsgCode(code<<32) | MsgCode(seq)
}

func (c MsgCode) Code() uint8 {
	return uint8((c >> 32) & 0xff)
}

func (c MsgCode) Sequence() uint32 {
	return uint32(c & 0xffffffff)
}

func (c MsgCode) IsResponse() bool {
	return ((c >> 40) & 1) == 1
}

func (c MsgCode) Uint64() uint64 {
	return uint64(c)
}

func (c MsgCode) String() string {
	return fmt.Sprintf("code: %v seq: %v isresp: %v", c.Code(), c.Sequence(), c.IsResponse())
}

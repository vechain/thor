// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package adapters

import (
	"bufio"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

// wsAddrPattern is a regex used to read the WebSocket address from the node's
// log
var wsAddrPattern = regexp.MustCompile(`ws://[\d.:]+`)

func matchWSAddr(str string) (string, bool) {
	if !strings.Contains(str, "WebSocket endpoint opened") {
		return "", false
	}

	return wsAddrPattern.FindString(str), true
}

// findWSAddr scans through reader r, looking for the log entry with
// WebSocket address information.
func findWSAddr(r io.Reader, timeout time.Duration) (string, error) {
	ch := make(chan string)

	go func() {
		s := bufio.NewScanner(r)
		for s.Scan() {
			addr, ok := matchWSAddr(s.Text())
			if ok {
				ch <- addr
			}
		}
		close(ch)
	}()

	var wsAddr string
	select {
	case wsAddr = <-ch:
		if wsAddr == "" {
			return "", errors.New("empty result")
		}
	case <-time.After(timeout):
		return "", errors.New("timed out")
	}

	return wsAddr, nil
}

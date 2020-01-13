// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// +build darwin

package muxdb

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"syscall"

	"github.com/syndtr/goleveldb/leveldb/storage"
)

func openLevelFileStorage(path string, readOnly, disablePageCache bool) (storage.Storage, error) {
	s, err := storage.OpenFile(path, readOnly)
	if err != nil {
		return nil, err
	}
	// only support darwin now
	if disablePageCache {
		if runtime.GOOS == "darwin" {
			return &storageNoPageCache{s}, nil
		}
		fmt.Fprintln(os.Stderr, "storageNoPageCache unsupported for OS", runtime.GOOS)
	}
	return s, nil
}

// storageNoPageCache is leveldb storage with sys cache disabled (darwin only).
type storageNoPageCache struct {
	storage.Storage
}

func (s *storageNoPageCache) Open(fd storage.FileDesc) (storage.Reader, error) {
	r, err := s.Storage.Open(fd)
	if err != nil {
		return nil, err
	}
	s.syscallSetNoCache(r)
	return r, nil
}

func (s *storageNoPageCache) Create(fd storage.FileDesc) (storage.Writer, error) {
	w, err := s.Storage.Create(fd)
	if err != nil {
		return nil, err
	}
	s.syscallSetNoCache(w)
	return w, nil
}

func (s *storageNoPageCache) syscallSetNoCache(o interface{}) {
	// extract os.File from fileWrap
	of := reflect.ValueOf(o).Elem().FieldByIndex([]int{0}).Interface().(*os.File)
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, of.Fd(), syscall.F_NOCACHE, 1)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "failed to set F_NOCACHE: %v\n", errno)
	}
}

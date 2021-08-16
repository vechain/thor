package main

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

const (
	rootKey              = "root"
	totalKeyCount        = 5000000
	readKeyCount         = 5000
	readNonExistKeyCount = 100000
	iterateCount         = 10000
)

type bench struct {
	path      string
	optimized bool
}

func (b *bench) openDB() (*muxdb.MuxDB, error) {
	return muxdb.Open(b.path, &muxdb.Options{
		TrieCacheSizeMB:        0,
		TrieRootCacheCapacity:  0,
		DisablePageCache:       true,
		OpenFilesCacheCapacity: 500,
		ReadCacheMB:            256,
		WriteBufferMB:          64,
	})
}

func (b *bench) Write(f func(put kv.PutFunc) error) error {
	db, err := b.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	root, commitNum, err := loadRoot(db)
	if err != nil {
		return err
	}

	if !root.IsZero() {
		return nil
	}

	if b.optimized {
		tr := db.NewSecureTrie("", thor.Bytes32{}, 0)
		count := 0

		if err := f(func(key, val []byte) error {
			if err := tr.Update(key, val, nil); err != nil {
				return err
			}
			if count > 0 && count%10000 == 0 {
				if _, err := tr.Commit(commitNum); err != nil {
					return err
				}
				commitNum++
			}
			count++
			return nil
		}); err != nil {
			return err
		}
		if root, err = tr.Commit(commitNum); err != nil {
			return err
		}
	} else {
		tr, err := trie.New(thor.Bytes32{}, db.LowStore())
		if err != nil {
			return err
		}
		count := 0

		if err := f(func(key, val []byte) error {
			if err := tr.TryUpdate(thor.Blake2b(key).Bytes(), val); err != nil {
				return err
			}
			if count > 0 && count%10000 == 0 {
				if _, err := tr.Commit(); err != nil {
					return err
				}
			}
			count++
			return nil
		}); err != nil {
			return err
		}
		if root, err = tr.Commit(); err != nil {
			return err
		}
	}
	return saveRoot(db, root, commitNum)
}

func (b *bench) Read(f func(get kv.GetFunc) error) error {
	db, err := b.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	root, commitNum, err := loadRoot(db)
	if err != nil {
		return err
	}

	if b.optimized {
		tr := db.NewSecureTrie("", root, commitNum)
		return f(func(key []byte) ([]byte, error) {
			v, _, err := tr.Get(key)
			return v, err
		})
	}

	tr, err := trie.New(root, db.LowStore())
	if err != nil {
		return err
	}
	return f(func(key []byte) ([]byte, error) {
		return tr.TryGet(thor.Blake2b(key).Bytes())
	})
}

func (b *bench) Iterate(n int) error {
	db, err := b.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	root, commitNum, err := loadRoot(db)
	if err != nil {
		return err
	}

	var iter trie.NodeIterator
	if b.optimized {
		iter = db.NewTrie("", root, commitNum).NodeIterator(nil, nil)
	} else {
		tr, err := trie.New(root, db.LowStore())
		if err != nil {
			return err
		}
		iter = tr.NodeIterator(nil)
	}

	for i := 0; i < n && iter.Next(true); i++ {
	}
	return iter.Error()
}

func (b *bench) Run() error {
	fmt.Println("fill", totalKeyCount, "keys ...")
	t := time.Now().UnixNano()
	if err := b.Write(func(put kv.PutFunc) error {
		for i := 0; i < totalKeyCount; i++ {
			var b4 [4]byte
			binary.BigEndian.PutUint32(b4[:], uint32(i))
			value := thor.Blake2b(thor.Blake2b(b4[:]).Bytes())
			if err := put(b4[:], value[:]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Println("elapse:", time.Duration(time.Now().UnixNano()-t))

	fmt.Println("read", readKeyCount, "keys ...")
	t = time.Now().UnixNano()
	if err := b.Read(func(get kv.GetFunc) error {
		for i := 0; i < readKeyCount; i++ {
			var b4 [4]byte
			binary.BigEndian.PutUint32(b4[:], uint32(i))
			if _, err := get(b4[:]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Println("elapse:", time.Duration(time.Now().UnixNano()-t))

	fmt.Println("read", readNonExistKeyCount, "non-exist keys ...")
	t = time.Now().UnixNano()
	if err := b.Read(func(get kv.GetFunc) error {
		for i := 0; i < readNonExistKeyCount; i++ {
			var b4 [4]byte
			binary.BigEndian.PutUint32(b4[:], uint32(i+totalKeyCount))
			if _, err := get(b4[:]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Println("elapse:", time.Duration(time.Now().UnixNano()-t))

	fmt.Println("iterate", iterateCount, "nodes ...")
	t = time.Now().UnixNano()
	if err := b.Iterate(iterateCount); err != nil {
		return err
	}
	fmt.Println("elapse:", time.Duration(time.Now().UnixNano()-t))
	return nil
}

type xroot struct {
	Root      thor.Bytes32
	CommitNum uint32
}

func loadRoot(db *muxdb.MuxDB) (thor.Bytes32, uint32, error) {
	val, err := db.NewStore("c2").Get([]byte(rootKey))
	if err != nil {
		if db.IsNotFound(err) {
			return thor.Bytes32{}, 0, nil
		}
		return thor.Bytes32{}, 0, err
	}
	var xr xroot
	rlp.DecodeBytes(val, &xr)
	return xr.Root, xr.CommitNum, nil
}

func saveRoot(db *muxdb.MuxDB, root thor.Bytes32, commitNum uint32) error {
	enc, _ := rlp.EncodeToBytes(&xroot{
		root,
		commitNum,
	})
	return db.NewStore("c2").Put([]byte(rootKey), enc)
}

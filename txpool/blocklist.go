// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vechain/thor/v2/thor"
)

// blocklist is a address list contains addresses that are blocked.
type blocklist struct {
	list map[thor.Address]bool
	lock sync.Mutex
}

// Load load list from local file.
func (bl *blocklist) Load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	newList, err := bl.readList(file)
	if err != nil {
		return err
	}

	bl.lock.Lock()
	bl.list = newList
	bl.lock.Unlock()

	return nil
}

// Save save list to local file.
func (bl *blocklist) Save(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var listToSave []thor.Address

	bl.lock.Lock()
	for addr := range bl.list {
		listToSave = append(listToSave, addr)
	}
	bl.lock.Unlock()

	for _, addr := range listToSave {
		if _, err := file.WriteString(addr.String() + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// Fetch fetch list from remote url.
func (bl *blocklist) Fetch(ctx context.Context, url string, eTag *string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if eTag != nil && *eTag != "" {
		req.Header.Add("if-none-match", *eTag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusNotModified {
		return nil
	}

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("status %v", resp.Status)
	}

	newList, err := bl.readList(resp.Body)
	if err != nil {
		return err
	}

	bl.lock.Lock()
	bl.list = newList
	bl.lock.Unlock()

	if eTag != nil {
		*eTag = resp.Header.Get("etag")
	}
	return nil
}

// Contains returns whether the given address is listed.
func (bl *blocklist) Contains(addr thor.Address) bool {
	bl.lock.Lock()
	defer bl.lock.Unlock()

	return bl.list[addr]
}

func (bl *blocklist) Len() int {
	bl.lock.Lock()
	defer bl.lock.Unlock()

	return len(bl.list)
}

func (bl *blocklist) readList(r io.Reader) (map[thor.Address]bool, error) {
	scanner := bufio.NewScanner(r)
	list := make(map[thor.Address]bool)

	for scanner.Scan() {
		addrStr := strings.TrimSpace(scanner.Text())
		if addrStr == "" {
			continue
		}
		addr, err := thor.ParseAddress(addrStr)
		if err != nil {
			return nil, err
		}
		list[addr] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func runBlocklistLoop(ctx context.Context, options Options, bl *blocklist) {
	path, url := options.BlocklistCacheFilePath, options.BlocklistFetchURL
	if path != "" {
		if err := bl.Load(path); err != nil {
			if !os.IsNotExist(err) {
				logger.Warn("blocklist load failed", "error", err, "path", path)
			}
		} else {
			logger.Debug("blocklist loaded", "len", bl.Len())
		}
	}
	if url == "" {
		return
	}

	var eTag string
	fetch := func() {
		if err := bl.Fetch(ctx, url, &eTag); err != nil {
			if err != context.Canceled {
				logger.Warn("blocklist fetch failed", "error", err, "url", url)
			}
			return
		}
		logger.Debug("blocklist fetched", "len", bl.Len())
		if path != "" {
			if err := bl.Save(path); err != nil {
				logger.Warn("blocklist save failed", "error", err, "path", path)
			} else {
				logger.Debug("blocklist saved")
			}
		}
	}

	fetch()
	for {
		delay := time.Second * time.Duration(rand.Int()%60+60) //#nosec G404
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			fetch()
		}
	}
}

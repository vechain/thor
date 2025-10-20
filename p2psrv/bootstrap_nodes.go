// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/vechain/thor/v2/p2psrv/tempdiscv5"
)

func fetchRemoteBootstrapNodes(ctx context.Context, remoteURL string) ([]*tempdiscv5.Node, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", remoteURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http fetch failed: statusCode=%d", resp.StatusCode)
	}

	nodes := []*tempdiscv5.Node{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if scanner.Text() != "" {
			disc, err := tempdiscv5.ParseNode(scanner.Text())
			if err != nil {
				return nil, err
			}

			nodes = append(nodes, disc)
		}
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return nodes, nil
}

package p2psrv

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/ethereum/go-ethereum/p2p/discv5"
)

func fetchRemoteBootstrapNodes(ctx context.Context, remoteURL string) ([]*discv5.Node, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", remoteURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	defer io.Copy(ioutil.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http fetch failed: statusCode=%d", resp.StatusCode)
	}

	nodes := []*discv5.Node{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if scanner.Text() != "" {
			disc, err := discv5.ParseNode(scanner.Text())
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

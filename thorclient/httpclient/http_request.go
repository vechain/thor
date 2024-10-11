// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vechain/thor/v2/thorclient/common"
)

func (c *Client) httpRequest(method, url string, payload io.Reader) ([]byte, error) {
	body, statusCode, err := c.rawHTTPRequest(method, url, payload)
	if err != nil {
		return nil, err
	}
	if !statusCodeIs2xx(statusCode) {
		return nil, fmt.Errorf("http error - Status Code %d - %s - %w", statusCode, body, common.ErrNot200Status)
	}
	return body, nil
}

func statusCodeIs2xx(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func (c *Client) rawHTTPRequest(method, url string, payload io.Reader) ([]byte, int, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating request: %w", err)
	}

	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.c.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("error reading response body: %w", err)
	}

	return responseBody, resp.StatusCode, nil
}

func (c *Client) httpGET(url string) ([]byte, error) {
	return c.httpRequest("GET", url, nil)
}

func (c *Client) httpPOST(url string, payload interface{}) ([]byte, error) {
	var data []byte

	if _, ok := payload.([]byte); ok {
		data = payload.([]byte)
	} else {
		var err error
		data, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal payload - %w", err)
		}
	}

	return c.httpRequest("POST", url, bytes.NewBuffer(data))
}

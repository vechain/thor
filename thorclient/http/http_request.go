package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vechain/thor/v2/thorclient/common"
)

func (c *Client) httpRequest(method, url string, payload io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http error - Status Code %d - %s - %w", resp.StatusCode, responseBody, common.Not200StatusErr)
	}
	return responseBody, nil
}

func (c *Client) httpGET(url string) ([]byte, error) {
	return c.httpRequest("GET", url, nil)
}

func (c *Client) httpPOST(url string, payload interface{}) ([]byte, error) {
	var data []byte
	var err error

	if _, ok := payload.([]byte); ok {
		data = payload.([]byte)
	} else {
		data, err = json.Marshal(payload)
	}

	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal payload - %w", err)
	}

	if string(data) == "[]" {
		return nil, fmt.Errorf("invalid nil marshalling")
	}

	return c.httpRequest("POST", url, bytes.NewBuffer(data))
}

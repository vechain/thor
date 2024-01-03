package client

import (
	"encoding/json"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"net/http"
	"strconv"
)

var (
	Default = NewThorClient("http://localhost:8669")
	Node1   = NewThorClient("http://localhost:8669")
	Node2   = NewThorClient("http://localhost:8679")
	Node3   = NewThorClient("http://localhost:8689")
)

type ThorClient struct {
	baseUrl string
}

func NewThorClient(baseUrl string) *ThorClient {
	return &ThorClient{baseUrl: baseUrl}
}

func (c *ThorClient) GetAccount(address string) (*accounts.Account, error) {
	return Get(c, "/accounts/"+address, new(accounts.Account))
}

func (c *ThorClient) GetExpandedBlock(number int32) (*blocks.JSONExpandedBlock, error) {
	return Get(c, "/blocks/"+strconv.Itoa(int(number))+"?expanded=true", new(blocks.JSONExpandedBlock))
}

func (c *ThorClient) GetCompressedBlock(number int32) (*blocks.JSONCollapsedBlock, error) {
	return Get(c, "/blocks/"+strconv.Itoa(int(number)), new(blocks.JSONCollapsedBlock))
}

func Get[T any](c *ThorClient, endpoint string, v *T) (*T, error) {

	resp, err := http.Get(c.baseUrl + endpoint)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

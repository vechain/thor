package testnode

import "github.com/vechain/thor/v2/thorclient"

// Client wraps thorclient.Client to add minting functionality
type Client struct {
	thorclient.ClientInterface
	mintFunc func() error // private field for minting
}

// MintTransactions provides minting functionality
func (c *Client) MintTransactions() error {
	if c.mintFunc == nil {
		return nil
	}
	return c.mintFunc()
}

// NewClient creates a new Client with minting capability
func NewClient(client *thorclient.Client, mintFunc func() error) *Client {
	return &Client{
		ClientInterface: client,
		mintFunc:        mintFunc,
	}
}

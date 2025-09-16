package main

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discv5"
	lru "github.com/hashicorp/golang-lru"
)

func pollMetrics(ctx context.Context, net *discv5.Network) {
	ticker := time.NewTicker(20 * time.Second)

	cache, err := lru.NewARC(100)
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			nodes := make([]*discv5.Node, 16)
			net.ReadRandomNodes(nodes)
			for _, n := range nodes {
				cache.Add(n.ID.String(), true)
			}
		}
	}
}

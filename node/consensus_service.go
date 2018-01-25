package node

import (
	"context"
	"fmt"
	"time"
)

func consensusService(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("++++++++++++++++++")
			return
		default:
			time.Sleep(1 * time.Second)
			fmt.Println("-----------------")
		}
	}
}

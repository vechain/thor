package node

import (
	"context"
	"fmt"
	"time"
)

func proposerService(ctx context.Context, bp *blockPool) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("proposerService exit")
			return
		default:
			time.Sleep(1 * time.Second)
			fmt.Println("-----------------")
		}
	}
}

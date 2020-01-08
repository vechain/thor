package node

// func (n *Node) epochRoundInfoLoop(ctx context.Context) {
// 	ticker := time.NewTicker(time.Second)
// 	defer ticker.Stop()

// 	launchTime := n.chain.GenesisBlock().Header().Timestamp()
// 	blockInterval := thor.BlockInterval
// 	epochInterval := thor.EpochInterval

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		case <-ticker.C:
// 			now := uint64(time.Now().Unix())
// 			r := (now - launchTime) / blockInterval
// 			e := (r - 1) / epochInterval

// 			// This is the place we update epoch number and beacon
// 			if uint32(e) > n.epochNum {
// 				n.mu.Lock()
// 				n.epochNum = uint32(e)
// 				beacon, err := n.cons.Beacon(n.epochNum)
// 				if err != nil {
// 					panic(err)
// 				}
// 				n.beacon = beacon
// 				n.mu.Unlock()
// 			}

// 			// This is the place we update round number and seed
// 			if uint32(r) > n.roundNum {
// 				n.mu.Lock()
// 				n.roundNum = uint32(r)
// 				n.seed = consensus.CompRoundSeed(n.beacon, n.roundNum)
// 				n.mu.Unlock()
// 			}
// 		}
// 	}
// }

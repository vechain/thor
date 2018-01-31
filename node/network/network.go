package network

import (
	"sync"

	"github.com/vechain/thor/block"
)

// Network 主题类
type Network struct {
	services []service
	mutex    *sync.Mutex
}

// New 工厂方法
func New() *Network {
	return &Network{
		services: nil,
		mutex:    new(sync.Mutex)}
}

// Join 加入网络
func (sb *Network) Join(svr service) {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	sb.services = append(sb.services, svr)
}

// Notify 广播
func (sb *Network) Notify(source service, block block.Block) {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	for _, svr := range sb.services {
		if svr.GetIP() != source.GetIP() {
			svr.UpdateBlockPool(block)
		}
	}
}

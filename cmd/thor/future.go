package main

import (
	"container/heap"

	"github.com/vechain/thor/block"
)

type futureBlock struct {
	f *future
}

func NewFutureBlock() *futureBlock {
	f := make(future, 0)
	heap.Init(&f)

	return &futureBlock{
		f: &f,
	}
}

func (fb *futureBlock) Pop() *block.Block {
	item := heap.Pop(fb.f)
	if item == nil {
		return nil
	}
	return item.(*block.Block)
}

func (fb *futureBlock) Push(blk *block.Block) {
	heap.Push(fb.f, blk)
}

type future []*block.Block

func (f future) Len() int {
	return len(f)
}

func (f future) Less(i, j int) bool {
	if len(f) < 2 {
		return false
	}
	return f[i].Header().Timestamp() < f[j].Header().Timestamp()
}

func (f future) Swap(i, j int) {
	if len(f) < 2 {
		return
	}
	f[i], f[j] = f[j], f[i]
}

func (f *future) Push(x interface{}) {
	*f = append(*f, x.(*block.Block))
}

func (f *future) Pop() interface{} {
	n := len(*f)
	if n == 0 {
		return nil
	}
	x := (*f)[n-1]
	*f = (*f)[0 : n-1]
	return x
}

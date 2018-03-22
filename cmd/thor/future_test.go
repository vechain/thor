package main

import (
	"testing"

	"github.com/vechain/thor/block"
)

func TestFuture(t *testing.T) {
	heap := NewFutureBlock()

	b1 := new(block.Builder).Timestamp(1).Build()
	b2 := new(block.Builder).Timestamp(2).Build()
	b3 := new(block.Builder).Timestamp(3).Build()

	heap.Push(b3)
	heap.Push(b1)
	heap.Push(b2)

	t.Log(heap.Pop())
	t.Log(heap.Pop())
	t.Log(heap.Pop())
	t.Log(heap.Pop())
}

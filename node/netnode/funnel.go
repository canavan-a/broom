package netnode

import (
	"time"
)

type BottleNeckQueue[T any] struct {
	head *QueueNode[T]
	tail *QueueNode[T]
}

type QueueNode[T any] struct {
	value  T
	isRoot bool
	next   *QueueNode[T]
}

func NewRootQueueNode[T any]() *QueueNode[T] {

	return &QueueNode[T]{
		isRoot: true,
	}
}

func NewBottleNeckQueue[T any]() *BottleNeckQueue[T] {
	node := NewRootQueueNode[T]()
	return &BottleNeckQueue[T]{
		head: node,
		tail: node,
	}
}

func (bn *BottleNeckQueue[T]) Add(value T) {
	node := &QueueNode[T]{
		value: value,
	}

	bn.head.next = node
	bn.head = node
}

func (bn *BottleNeckQueue[T]) HasNext() bool {
	return bn.tail.next != nil
}

func (bn *BottleNeckQueue[T]) Pop() (value T, found bool) {
	first := bn.tail.next
	if first == nil {
		return
	}

	value = first.value
	bn.tail.next = first.next
	if bn.tail.next == nil {
		bn.head = bn.tail
	}

	return value, true
}

func (bn *BottleNeckQueue[T]) Clear() {
	bn.tail.next = nil
	bn.head = bn.tail
}

type Funnel[T any] struct {
	Ingress  chan T
	Egress   chan T
	FlowRate time.Duration

	bottleNeck *BottleNeckQueue[T]
}

func NewFunnel[T any](rate time.Duration) *Funnel[T] {

	funnel := &Funnel[T]{
		Ingress:  make(chan T, 10),
		Egress:   make(chan T, 10),
		FlowRate: rate,

		bottleNeck: NewBottleNeckQueue[T](),
	}

	go funnel.loop()
	return funnel
}

func (fun Funnel[T]) loop() {

	ticker := time.NewTicker(fun.FlowRate)

	for {
		select {
		case value := <-fun.Ingress:
			fun.bottleNeck.Add(value)
		case <-ticker.C:
			value, found := fun.bottleNeck.Pop()
			if found {
				fun.Egress <- value
			}
		}
	}
}

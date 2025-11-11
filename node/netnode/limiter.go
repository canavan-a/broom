package netnode

import (
	"time"
)

type Limiter[T any] struct {
	Fun       *Funnel[T]
	Publisher func(T)
	Validator func(T) bool
}

func NewLimiter[T any](rate time.Duration, publisher func(T), validator func(T) bool) *Limiter[T] {

	limiter := &Limiter[T]{
		Fun:       NewFunnel[T](rate),
		Publisher: publisher,
		Validator: validator,
	}

	go limiter.listen()

	return limiter
}

func (li Limiter[T]) Publish(value T) {
	li.Fun.Ingress <- value

}

func (li Limiter[T]) listen() {
	for {
		value := <-li.Fun.Egress

		if li.Validator(value) {
			li.Publisher(value)
		}

	}
}

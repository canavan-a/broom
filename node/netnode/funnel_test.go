package netnode

import (
	"fmt"
	"testing"
)

func TestBottleNeckQueue(t *testing.T) {
	qn := NewRootQueueNode[string]()

	if qn.next != nil {
		panic("invalid")
	}

	bn := NewBottleNeckQueue[string]()

	bn.Add("aidan")
	bn.Add("maryrose")
	bn.Add("andrew")

	for bn.HasNext() {
		value, _ := bn.Pop()
		fmt.Println(value)

	}
	panic("hello")

}

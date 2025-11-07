package main

import (
	"fmt"
	"testing"

	"github.com/canavan-a/broom/node/netnode"
)

func TestMiner(t *testing.T) {
	m := NewMiner("sldkjfklsjdklfjlk", "node.broomledger.com", 3)

	res, err := RequestPool[any, netnode.HashHeight](m, nil, Get, "highest_block")
	if err != nil {
		panic(err)
	}

	fmt.Println(res)

	resStr, err := RequestPool[any, string](m, nil, Get, "version")
	if err != nil {
		panic(err)
	}

	fmt.Println(resStr)
}

package main

import (
	"fmt"

	"github.com/canavan-a/broom/node/netnode"
)

func main() {
	fmt.Println("hello world")

	ex := netnode.NewExecutor(
		"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==",
		"Hello world!!!",
		"",
		"",
	)

	ex.Start("192.168.1.168")
}

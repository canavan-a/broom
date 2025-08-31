package netnode

import (
	"fmt"
	"testing"
	"time"
)

func TestGenerateGenesisHash(t *testing.T) {
	txns := make(map[string]Transaction)
	txns[GENESIS] = GENESIS_TXN

	var GENESIS_BLOCK = Block{
		Timestamp:    time.Date(2001, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(),
		Transactions: txns,
	}

	GENESIS_BLOCK.SignHash()

	fmt.Println(GENESIS_BLOCK.Hash)
}

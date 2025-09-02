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

func TestValidateNonce(t *testing.T) {

	txns := []Transaction{
		{
			Nonce: 8,
		},
		{
			Nonce: 9,
		},
		{
			Nonce: 10,
		},
		{
			Nonce: 11,
		},
	}

	currentNonce, err := ValidateNonce(txns, 7)
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	if currentNonce != 11 {
		t.Fail()
	}

}

func TestValidateNonceFail(t *testing.T) {

	txns := []Transaction{
		{
			Nonce: 8,
		},
		{
			Nonce: 9,
		},
		{
			Nonce: 10,
		},
		{
			Nonce: 12,
		},
	}

	currentNonce, err := ValidateNonce(txns, 7)
	if err == nil {
		t.Error(err)
		t.Fail()
	}

	if currentNonce != 0 {
		t.Fail()
	}

}

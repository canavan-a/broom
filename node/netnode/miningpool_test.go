package netnode

import (
	"fmt"
	"testing"
)

func getNonce(h string) int64 {
	return 1
}

func TestMiningPool(t *testing.T) {

	// 	func InitMiningPool(
	// 		taxPercent int64,
	// 		blockPayout int64,
	// 		nodeMiningAddress string,
	// 		txnChan chan Transaction,
	// 		poolNote string,
	// 		privateKey string,
	// 		getNonce func(address string) int64,
	// )
	//
	fmt.Println("testing")

	txnChan := make(chan Transaction)

	mp := InitMiningPool(8, 10_000, "my address", txnChan, "test note", "my private key", getNonce)
	fmt.Println(mp)

	mp.AddWorkProof("hello hello hello", Block{Hash: "lsdkfjlkdafjs", Height: 8882})

	err := mp.BackupWorkLog()
	if err != nil {
		panic(err)
	}
	fmt.Println(mp)
	// t.Error("invalid")
}

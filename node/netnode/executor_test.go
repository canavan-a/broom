package netnode

import (
	"fmt"
	"testing"
	"time"
)

func TestExecutor(t *testing.T) {
	ex := NewExecutor([]string{}, "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==", "hello", true)

	ex.RunMiningLoop()
}

func TestFork(t *testing.T) {
	ex := NewExecutor([]string{}, "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==", "hello", true)

	// go ex.RunMiningLoop()

	WaitForBlock(3, ex)

}

func WaitForBlock(blockHeight int, ex *Executor) {

	for {
		blockHash, found := ex.database.GetFirstBlockByHeight(blockHeight)
		if found {
			fmt.Println("FOUND 3", blockHash)

			block, found := ex.database.GetBlock(blockHash, int64(blockHeight))
			if !found {
				panic("block not found, but previously found")
			}

			// currentLedger, found := ex.database.GetLedgerAt(blockHash, block.Height)
			// if !found {
			// 	panic("ledger not found, but block found")
			// }

			blockChanLocal := make(chan Block)
			doneChanLocal := make(chan struct{})

			fmt.Println()
			block.MineWithWorkers(DEFAULT_MINING_THRESHOLD, 2, blockChanLocal, doneChanLocal)

			solution := <-blockChanLocal
			close(doneChanLocal)
			fmt.Println("Auxilery block mined")
			fmt.Println(solution)

			time.Sleep(4 * time.Second)
			ex.database.ReceiveBlock(solution)

			return
		}
		time.Sleep(1 * time.Second)
	}
}

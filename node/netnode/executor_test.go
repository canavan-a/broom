package netnode

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestExecutor(t *testing.T) {
	ex := NewExecutor(
		"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==",
		"hello",
		"",
		"",
	)

	ex.RunMiningLoop(t.Context(), 1)
}

func TestFork(t *testing.T) {
	ledgerDir := "ledger"
	broombaseDir := "broombase"
	err := os.Remove(ledgerDir)
	if err != nil {
		// panic(err)
	}
	err = os.Remove(broombaseDir)
	if err != nil {
		// panic(err)
	}

	ex := NewExecutor(
		"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==",
		"hello",
		broombaseDir,
		ledgerDir,
	)

	go ex.RunMiningLoop(t.Context(), 1)

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

			blockChanLocal := make(chan Block)
			doneChanLocal := make(chan struct{})

			fmt.Println()
			block.MineWithWorkers(context.Background(), DEFAULT_MINING_THRESHOLD, 2, blockChanLocal, doneChanLocal)

			solution := <-blockChanLocal
			close(doneChanLocal)
			fmt.Println("Auxilery block mined")
			fmt.Println(solution)

			time.Sleep(4 * time.Second)
			ex.database.ReceiveBlock(solution)

			blockChanLocal = make(chan Block)
			doneChanLocal = make(chan struct{})

			solution.PreviousBlockHash = solution.Hash
			solution.Height = solution.Height + 1

			solution.MineWithWorkers(context.Background(), DEFAULT_MINING_THRESHOLD, 2, blockChanLocal, doneChanLocal)

			solution2 := <-blockChanLocal
			close(doneChanLocal)

			ex.database.ReceiveBlock(solution2)

			blockChanLocal = make(chan Block)
			doneChanLocal = make(chan struct{})

			solution2.PreviousBlockHash = solution2.Hash
			solution2.Height = solution2.Height + 1

			solution2.MineWithWorkers(context.Background(), DEFAULT_MINING_THRESHOLD, 2, blockChanLocal, doneChanLocal)

			solution3 := <-blockChanLocal
			close(doneChanLocal)

			ex.database.ReceiveBlock(solution3)

			return
		}
		time.Sleep(1 * time.Second)
	}
}

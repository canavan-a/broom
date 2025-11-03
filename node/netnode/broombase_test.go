package netnode

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestGenerateGenesisHash(t *testing.T) {

	GENESIS_TXN.Sign("")

	txns := make(map[string]Transaction)
	txns[GENESIS] = GENESIS_TXN

	var GENESIS_BLOCK = Block{
		Height:       GENESIS_BLOCK_HEIGHT,
		Timestamp:    time.Date(2001, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(),
		Transactions: txns,
	}

	GENESIS_BLOCK.SignHash()

	fmt.Println(GENESIS_BLOCK.Hash)

	// err := bb.AddBlock(&GENESIS_BLOCK)
	// if err != nil {
	// 	panic(err)
	// }
	//
	// txns := make(map[string]Transaction)
	// txns[GENESIS] = GENESIS_TXN

	// var GENESIS_BLOCK = Block{
	// 	Height:       GENESIS_BLOCK_HEIGHT,
	// 	Timestamp:    time.Date(2001, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(),
	// 	Transactions: txns,
	// }

	// GENESIS_BLOCK.SignHash()

	// fmt.Println(GENESIS_BLOCK.Hash)
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

func TestGetHighestBlock(t *testing.T) {
	bb := &Broombase{
		dir: "broombase",
		mut: sync.RWMutex{},
	}

	height, hash, err := bb.GetHighestBlock()
	if err != nil {
		t.Fail()
	}

	fmt.Println(height, hash)
}

func TestMinePoW(t *testing.T) {

	bb := InitBroombaseWithDir("broombase_test", "ledger_test")

	if bb.Ledger.Balances[GENESIS_TXN.To] != STARTING_PAYOUT {

		// t.Fail()
		// return
	}

	txn := generateTransaction()
	valid, err := txn.ValidateSig()
	if err != nil {
		t.Error(err)
		t.Fail()
		return
	}
	if !valid {
		t.Error("could not verify sig")
		t.Fail()
		return
	}

	easyMiningTarget := DEFAULT_MINING_THRESHOLD

	myAddress := txn.From

	block := NewBlock(myAddress, "test mining a block!", bb.Ledger.BlockHash, bb.Ledger.BlockHeight+1, int64(bb.Ledger.CalculateCurrentReward()))
	block._add(txn)

	done := make(chan struct{})

	solution := make(chan Block)

	block.MineWithWorkers(context.Background(), easyMiningTarget, 10, solution, done, make(map[string]func(b Block)))

	fmt.Println("block height: ", block.Height)

	sol := <-solution
	close(done)

	time.Sleep(2 * time.Second)
	fmt.Println("Completed")
	fmt.Println(sol.Hash)

	err = bb.Ledger.ValidateBlock(sol)
	if err != nil {
		fmt.Println("FAILED TO VALIDATE BLOCK")
		fmt.Println(err.Error())
		t.Error()
		t.Fail()
		return
	}

	err = bb.ValidateBlockHeaders(block)
	if err != nil {
		fmt.Println("FAILED TO VALIDATE BLOCK HEADERS")
		t.Error()
		t.Fail()
		return
	}

	// store the valid block
	err = bb.AddAndSync(&sol)
	if err != nil {
		fmt.Println("FAILED TO STORE BLOCK")
		t.Error()
		t.Fail()
		return
	}

	fmt.Println("block added to ledger:")
	fmt.Println(bb.Ledger.BlockHash)
	fmt.Println(bb.Ledger.BlockHeight)
	fmt.Println("Balances: ")
	fmt.Println(bb.Ledger.Balances)
	fmt.Println(bb.Ledger.Nonces)
	fmt.Println(bb.Ledger.Pending)

}

func generateTransaction() (txn Transaction) {

	txn = Transaction{
		Sig:      "9ce786d7bcbd89c354ce1cf60af7feb42871b687940185ecf3a6030c58b2ffd7.2453dbbe27e8ff2049417ea8911270cbac99a209a437ea15c9c3120e2283442e",
		Coinbase: false,
		Note:     "This is a test txn",
		Nonce:    1,
		To:       "nobody to go to",
		From:     "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==",
		Amount:   3600,
	}

	return
}

func TestSignTransaction(t *testing.T) {
	// this function only works if private key is supplied
	privkey := ""
	txn := generateTransaction()
	err := txn.Sign(privkey)
	if err != nil {
		t.Fail()
	}
	fmt.Println(txn.Sig)
}

// FORK NOTES

// => mining blocks
//   => new block comes in (no height condition)
//   	=> get a ledger form the file system (previous block previous height)
//         => validate block against this ledger
//				=> accumulate this ledger and save the new ledger to disk
//
// 		=> put block in storage
//
// How do we validate the block? without syncing?

//

func TestAdjustMiningDiff(t *testing.T) {
	ledger := &Ledger{
		BlockTimeDifficulty: []BlockTimeDifficulty{
			{
				Height:     0,
				Time:       parseTime("2001-11-10T19:00:00-05:00"),
				Difficulty: "0fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			},
			{
				Height:     1,
				Time:       parseTime("2025-09-19T14:46:26-04:00"),
				Difficulty: "0fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			},
			{
				Height:     2,
				Time:       parseTime("2025-09-19T14:46:28-04:00"),
				Difficulty: "03ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			},
			{
				Height:     3,
				Time:       parseTime("2025-09-19T14:46:49-04:00"),
				Difficulty: "00ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			},
			{
				Height:     4,
				Time:       parseTime("2025-09-19T14:47:35-04:00"),
				Difficulty: "004296efb8f899e55d39b602f5a4411c1d986a8b1927f4296efb8f899e55d39b",
			},
			{
				Height:     5,
				Time:       parseTime("2025-09-19T14:50:25-04:00"),
				Difficulty: "00123e64bef373ce9ecd2e5bfd297e8cf692f69d578cb31ce2ca2e56ccb1d0c4",
			},
			// {
			// 	Height:     6,
			// 	Time:       parseTime("2025-09-19T15:01:24-04:00"),
			// 	Difficulty: "00048f992fbcdcf3a7b34b96ff4a5fa33da4bda755e32cc738b28b95b32c7431",
			// },
		},
	}

	thresh := ledger._calculateNewMiningThreshold()
	fmt.Println(thresh)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

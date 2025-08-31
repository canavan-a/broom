package netnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const GENESIS = "genesis"
const BROOMBASE_DEFAULT_DIR = "broombase"
const GENESIS_BLOCK_HEIGHT = 0

type Broombase struct {
	mut    sync.RWMutex
	dir    string
	ledger *Ledger
}

type PendingBalance struct {
	To          string `json:"to"`
	Amount      int64  `json:"amount"`
	UnlockLevel int64  `json:"unlockLevel"`
}

type Ledger struct {
	mut         sync.RWMutex
	BlockHeight int64            `json:"blockHeight"`
	Balances    map[string]int64 `json:"balances"` // track receiver balances
	Nonces      map[string]int64 `json:"nonces"`   // track sender nonce values of transactions
	Pending     []PendingBalance `json:"pending"`
}

var GENESIS_HASH = "b6723616b2a8fe417b2c3c47ca0fada3e433148a061da2cd6c15513630cfc3ad"

var GENESIS_TXN = Transaction{
	Coinbase: true,
	To:       "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==",
	Note:     "It is written, my house will be called a house of prayer, but you are making it a den of thieves!",
	Amount:   STARTING_PAYOUT,
}

// dir can be empty string
func NewBroombase() *Broombase {
	return InitBroombaseWithDir("")
}

func InitBroombaseWithDir(dir string) *Broombase {

	bb := &Broombase{
		mut: sync.RWMutex{},
		ledger: &Ledger{
			mut:         sync.RWMutex{},
			BlockHeight: 0,
			Balances:    make(map[string]int64),
			Nonces:      make(map[string]int64),
			Pending:     make([]PendingBalance, 0),
		},
	}

	dir = strings.ToLower(dir)
	if dir == "" {
		dir = BROOMBASE_DEFAULT_DIR
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755) // create if missing
	}

	bb.dir = dir

	// see if we have genesis block
	_, found := bb.GetBlock(GENESIS_HASH, GENESIS_BLOCK_HEIGHT)
	if !found {

		txns := make(map[string]Transaction)
		txns[GENESIS] = GENESIS_TXN

		var GENESIS_BLOCK = Block{
			Timestamp:    time.Date(2001, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(),
			Transactions: txns,
		}

		GENESIS_BLOCK.SignHash()
		bb.AddBlock(&GENESIS_BLOCK)

	}

	highestBlock := bb.getHighestBlock()

	err := bb.SyncLedger(highestBlock)
	if err != nil {
		panic(err)
	}

	return bb

}

func (bb *Broombase) getHighestBlock() int64 {
	return 0
}

func (bb *Broombase) AddBlock(block *Block) error {

	if !block.ValidateHash() {
		return errors.New("block error: invalid hash")
	}

	_, found := bb.GetBlock(block.Hash, block.Height)
	if found {
		return errors.New("block error: block already exists")
	}

	previousBlock, found := bb.GetBlock(block.PreviousBlockHash, block.Height-1)
	if !found {
		return errors.New("block error: previous block does not exist")
	}

	// validate timestamp
	if block.GetTimestampTime().Compare(previousBlock.GetTimestampTime()) == -1 {
		return errors.New("block error: time is before previous block")
	}
	if block.GetTimestampTime().Compare(time.Now()) == 1 {
		return errors.New("block error: block time is in the future")
	}

	// TODO: validate transactions,
	// double spend,
	// coinbase amount,
	// coinbase validity (one per block)

	err := bb.StoreBlock(*block)
	if err != nil {
		return err
	}

	// TODO: Update ledger

	if bb.ledger.BlockHeight+1 != block.Height {
		err := bb.SyncLedger(block.Height - 1)
		if err != nil {
			return err
		}
	}

	return nil
}

// files are stored as height_hash.broom
func (bb *Broombase) GetBlock(hash string, height int64) (block Block, found bool) {
	bb.mut.RLock()
	defer bb.mut.RUnlock()

	path := fmt.Sprintf("%s/%d_%s.broom", bb.dir, height, hash)
	data, err := os.ReadFile(path)
	if err != nil {
		// file not found or read error
		return Block{}, false
	}

	var b Block
	err = json.Unmarshal(data, &b)
	if err != nil {
		return Block{}, false
	}

	return b, true
}

func (bb *Broombase) StoreBlock(block Block) error {
	bb.mut.Lock()
	defer bb.mut.Unlock()
	data, err := json.Marshal(block)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/%d_%s.broom", bb.dir, block.Height, block.Hash)
	return os.WriteFile(path, data, 0644)

}

func (bb *Broombase) SyncLedger(blockHeight int64) error {

	return nil
}

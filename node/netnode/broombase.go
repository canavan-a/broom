package netnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

const GENESIS = "genesis"
const BROOMBASE_DEFAULT_DIR = "broombase"
const GENESIS_BLOCK_HEIGHT = 0
const DEFAULT_MINING_THRESHOLD = "00000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

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
	mut                 *sync.RWMutex
	MiningThresholdHash string           `json:"miningThreshold"`
	BlockHeight         int64            `json:"blockHeight"`
	BlockHash           string           `json:"blockHash"`
	Balances            map[string]int64 `json:"balances"` // track receiver balances
	Nonces              map[string]int64 `json:"nonces"`   // track sender nonce values of transactions
	Pending             []PendingBalance `json:"pending"`
}

var GENESIS_HASH = "b6723616b2a8fe417b2c3c47ca0fada3e433148a061da2cd6c15513630cfc3ad"

var GENESIS_TXN = Transaction{
	Coinbase: false, // skip vesting schedule
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
			mut:                 &sync.RWMutex{},
			MiningThresholdHash: DEFAULT_MINING_THRESHOLD,
			BlockHeight:         0,
			Balances:            make(map[string]int64),
			Nonces:              make(map[string]int64),
			Pending:             make([]PendingBalance, 0),
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
			Height:       GENESIS_BLOCK_HEIGHT,
			Timestamp:    time.Date(2001, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(),
			Transactions: txns,
		}

		GENESIS_BLOCK.SignHash()
		bb.AddBlock(&GENESIS_BLOCK)

	}

	highestBlock, highestBlockHash := bb.getHighestBlock()

	err := bb.SyncLedger(highestBlock, highestBlockHash)
	if err != nil {
		panic(err)
	}

	return bb

}

func (bb *Broombase) getHighestBlock() (height int64, hash string) {
	// TODO!!!!!!
	return 0, ""
}

func (bb *Broombase) AddBlock(block *Block) error {

	if !block.ValidateHash() {
		return errors.New("block error: invalid hash")
	}

	_, found := bb.GetBlock(block.Hash, block.Height)
	if found {
		return errors.New("block error: block already exists")
	}

	if block.Height != GENESIS_BLOCK_HEIGHT {
		previousBlock, found := bb.GetBlock(block.PreviousBlockHash, block.Height-1)
		if !found {
			return errors.New("block error: previous block does not exist")
		}

		// validate timestamp
		if block.GetTimestampTime().Compare(previousBlock.GetTimestampTime()) == -1 {
			return errors.New("block error: time is before previous block")
		}

		// this sort of doesnt make sense because
		if block.GetTimestampTime().Compare(time.Now()) == 1 {
			return errors.New("block error: block time is in the future")
		}

		// TODO: validate transactions,
		// double spend,
		// coinbase amount,
		// coinbase validity (one per block)
	} else {
		if block.Hash != GENESIS_HASH {
			return errors.New("invalid genesis block hash, attemp made to attack genesis block")
		}
	}

	err := bb.StoreBlock(*block)
	if err != nil {
		return err
	}

	// TODO: Update ledger

	if bb.ledger.BlockHeight+1 != block.Height {
		err := bb.SyncLedger(block.Height, block.Hash)
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

// TDO: implement snapshots where we can jump to pick like n blocks ago
func (bb *Broombase) SyncLedger(blockHeight int64, blockHash string) error {

	//clear ledger (can't sync with old data)
	bb.ledger.Clear()

	// keep as much data not in memory as possible
	type heightHash struct {
		height int64
		hash   string
	}

	fingerPrint := make([]heightHash, blockHeight+1)

	// trace backwards through chain until genesis
	for blockHeight >= 0 {

		block, found := bb.GetBlock(blockHash, blockHeight)
		if !found {
			return errors.New("SyncLedger: could not find block")
		}
		fingerPrint[blockHeight] = heightHash{
			height: block.Height,
			hash:   block.Hash,
		}

		blockHeight = block.Height - 1
		blockHash = block.PreviousBlockHash
	}

	for _, finger := range fingerPrint {
		block, found := bb.GetBlock(finger.hash, finger.height)
		if !found {
			return errors.New("SyncLedger: could not find block")
		}

		if block.Height != GENESIS_BLOCK_HEIGHT { // DO NOT VALIDATE GENESIS BLOCK
			bb.ledger.ValidateBlock(block)
		}

		bb.ledger.Accumulate(block)
	}

	return nil
}

// block must validate against current ledger
func (l *Ledger) ValidateBlock(block Block) error {

	l.mut.RLock()
	defer l.mut.RUnlock()

	if block.CalculateHash() != block.Hash {
		return errors.New("ValidateBlock: signed hash is incorrect for block")
	}

	if !CompareHash(block.Hash, l.MiningThresholdHash) {
		return errors.New("ValidateBlock: block has not found solution")
	}

	// vlaidate height
	if block.Height-l.BlockHeight != 1 {
		return errors.New("ValidateBlock: cannot validate block on incorrect height increment")
	}

	// vlaidate hash
	if block.PreviousBlockHash != l.BlockHash {
		return errors.New("ValidateBlock: cannot validate block on incorrect previous block")
	}

	accountTransactions := make(map[string][]Transaction)

	coinbaseTxns := 0
	for _, txn := range block.Transactions {

		// check coinbase txns
		if txn.Coinbase {
			coinbaseTxns++
		}
		if coinbaseTxns > 1 {
			// coinbase txns can be 0
			return errors.New("ValidateBlock: too many coinbase txns")
		}

		if txn.Coinbase {
			err := l.ValidateCoinbaseTxn(txn, block.Height)
			if err != nil {
				return err
			}
		} else {
			valid, err := txn.ValidateSig()
			if !valid || err != nil {
				return errors.New("ValidateBlock: could not validate transaction")
			}

			accountTransactions[txn.From] = append(accountTransactions[txn.From], txn)

		}

	}

	for from, txnGroup := range accountTransactions {
		// check nonces and sum

		// check sum of txns is valid
		var sendAmount int64
		for _, txn := range txnGroup {
			sendAmount += txn.Amount
		}

		if l.Balances[from]-sendAmount < 0 {
			return errors.New("invalid ")
		}

		// check nonces
		_, err := ValidateNonce(txnGroup, l.Nonces[from])
		if err != nil {
			return err
		}

	}

	return nil

}

func ValidateNonce(txns []Transaction, lastNonce int64) (curentNonce int64, err error) {
	var nonces []int64

	for _, txn := range txns {
		nonces = append(nonces, txn.Nonce)
	}

	for i := range nonces {
		found := slices.Contains(nonces, lastNonce+int64(i)+1)
		if !found {
			return 0, errors.New("invalid txns nonces")
		}
	}

	slices.Sort(nonces)
	slices.Reverse(nonces)

	return nonces[0], nil

}

// Add halving rule, or gambling rules
func (l *Ledger) ValidateCoinbaseTxn(coinbaseTxn Transaction, currentBlock int64) error {
	if coinbaseTxn.Amount != STARTING_PAYOUT {
		return errors.New("invalid coinbase txn")
	}
	return nil
}

// need to remove validity checking, assumes we have a valid block
func (l *Ledger) Accumulate(b Block) {
	l.mut.Lock()
	defer l.mut.Unlock()

	// increment ledger
	l.BlockHeight = b.Height
	l.BlockHash = b.Hash
	l.MiningThresholdHash = l.CalculateNewMiningThreshold()

	accountTransactions := make(map[string][]Transaction)

	for _, txn := range b.Transactions {
		if txn.Coinbase {
			// add coinbase to pending txns
			// let it vest
			l.Pending = append(l.Pending, PendingBalance{
				To:          txn.To,
				Amount:      txn.Amount,
				UnlockLevel: COINBASE_VESTING_BLOCK_NUMBER + b.Height,
			})
		} else {
			// add coinbase txns to pending
			l.Balances[txn.From] = l.Balances[txn.From] - txn.Amount
			l.Balances[txn.To] = l.Balances[txn.To] + txn.Amount

			accountTransactions[txn.From] = append(accountTransactions[txn.From], txn)
		}
	}

	// settle nonces
	for from, txnGroup := range accountTransactions {

		// check nonces
		nonce, _ := ValidateNonce(txnGroup, l.Nonces[from]) // this fails for GENESIS block, resulting in 0 (undefined behavior)

		l.Nonces[from] = nonce

	}

	// find all pending txns that have vested and settle them
	var newPending []PendingBalance
	for _, pending := range l.Pending {
		if pending.UnlockLevel == b.Height {
			l.Balances[pending.To] = l.Balances[pending.To] + pending.Amount
		} else {
			newPending = append(newPending, pending)
		}
	}

	// replace underlying slice with the remainder
	l.Pending = newPending

}

func (l *Ledger) CalculateNewMiningThreshold() string {
	return DEFAULT_MINING_THRESHOLD
}

func (l *Ledger) Clear() {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.BlockHeight = -1
	l.Balances = make(map[string]int64)
	l.Nonces = make(map[string]int64)
	l.Pending = l.Pending[:0]
}

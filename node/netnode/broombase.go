package netnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const GENESIS = "genesis"
const BROOMBASE_DEFAULT_DIR = "broombase"
const LEDGER_DEFAULT_DIR = "ledger"

const GENESIS_BLOCK_HEIGHT = 0

const BLOCK_SPEED_AVERAGE_WINDOW = 50
const DEFAULT_MINING_THRESHOLD = "0fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

const TARGET_BLOCK_TIME = 60
const ALPHA_FACTOR = 4

type Broombase struct {
	mut       sync.RWMutex
	dir       string
	ledgerDir string
	ledger    *Ledger
}

type PendingBalance struct {
	To          string `json:"to"`
	Amount      int64  `json:"amount"`
	UnlockLevel int64  `json:"unlockLevel"`
}

type BlockTimeDifficulty struct {
	Height     int64     `json:"height"`
	Time       time.Time `json:"time"`
	Difficulty string    `json:"difficulty"`
}

type Ledger struct {
	mut                 *sync.RWMutex         `json:"-"`
	MiningThresholdHash string                `json:"miningThreshold"`
	BlockHeight         int64                 `json:"blockHeight"`
	BlockHash           string                `json:"blockHash"`
	Balances            map[string]int64      `json:"balances"` // track receiver balances
	Nonces              map[string]int64      `json:"nonces"`   // track sender nonce values of transactions
	BlockTimeDifficulty []BlockTimeDifficulty `json:"blockTime"`
	Pending             []PendingBalance      `json:"pending"`
}

var GENESIS_HASH = "fd1cb7926a4447454a00a3c5c75126ad8ec053bc5ba61d706ca687428c8af5ac"

var GENESIS_TXN = Transaction{
	Coinbase: false, // skip vesting schedule
	To:       "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEW7I9+SUHOW28jjABqnpO76tqwG/nCG/jMMPuUfIQryMPlCdxPwUrSP49ioqYZAf2kXrXQ7MQE891OXBTSpvlsA==",
	Note:     "Matthew 5:16",
	Amount:   STARTING_PAYOUT,
}

// dir can be empty string
func NewBroombase() *Broombase {
	return InitBroombaseWithDir("", "")
}

func InitBroombaseWithDir(dir, ledgerDir string) *Broombase {

	bb := &Broombase{
		mut: sync.RWMutex{},
		ledger: &Ledger{
			mut:                 &sync.RWMutex{},
			MiningThresholdHash: DEFAULT_MINING_THRESHOLD,
			BlockHeight:         0,
			Balances:            make(map[string]int64),
			Nonces:              make(map[string]int64),
			BlockTimeDifficulty: make([]BlockTimeDifficulty, 0),
			Pending:             make([]PendingBalance, 0),
		},
	}

	dir = strings.ToLower(dir)
	if dir == "" {
		dir = BROOMBASE_DEFAULT_DIR
	}

	ledgerDir = strings.ToLower(ledgerDir)
	if ledgerDir == "" {
		ledgerDir = LEDGER_DEFAULT_DIR
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755) // create if missing
	}

	if _, err := os.Stat(ledgerDir); os.IsNotExist(err) {
		_ = os.MkdirAll(ledgerDir, 0755) // create if missing
	}

	bb.dir = dir
	bb.ledgerDir = ledgerDir

	// see if we have genesis block
	_, found := bb.GetBlock(GENESIS_HASH, GENESIS_BLOCK_HEIGHT)
	if !found {
		fmt.Println("GENESIS BLOCK NOT FOUND")
		txns := make(map[string]Transaction)
		txns[GENESIS] = GENESIS_TXN

		var GENESIS_BLOCK = Block{
			Height:       GENESIS_BLOCK_HEIGHT,
			Timestamp:    time.Date(2001, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(),
			Transactions: txns,
		}

		GENESIS_BLOCK.SignHash()
		fmt.Println(GENESIS_BLOCK.Hash)

		err := bb.ValidateBlockHeaders(&GENESIS_BLOCK)
		if err != nil {
			panic(err)
		}
		err = bb.AddAndSync(&GENESIS_BLOCK)
		if err != nil {
			panic(err)
		}

	}

	highestBlock, highestBlockHash, err := bb.getHighestBlock()
	if err != nil {
		panic(err)
	}

	ledger, found := bb.GetLedgerAt(highestBlockHash, highestBlock)
	if !found {
		err = bb.SyncLedger(highestBlock, highestBlockHash)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println("setting ledger")
		bb.ledger = ledger
		bb.ledger.mut = &sync.RWMutex{}
	}

	return bb

}

// reconciles forks, does not TRACK them
func (bb *Broombase) getHighestBlock() (height int64, hash string, err error) {
	bb.mut.RLock()
	defer bb.mut.RUnlock()

	contents, err := os.ReadDir(bb.dir)
	if err != nil {
		return
	}

	type HeightEntry struct {
		height int64
		hash   string
	}

	var formattedContents []HeightEntry

	for _, entry := range contents {
		splitEntry := strings.Split(entry.Name(), "_")

		h, err := strconv.Atoi(splitEntry[0])
		if err != nil {
			return 0, "", err
		}
		hashValue := strings.Split(splitEntry[1], ".broom")

		formattedContents = append(formattedContents, HeightEntry{
			height: int64(h),
			hash:   hashValue[0],
		})

	}

	slices.SortFunc(formattedContents, func(a HeightEntry, b HeightEntry) int {
		return int(b.height) - int(a.height)
	})

	var tiedEntries []HeightEntry

	highestHeight := formattedContents[0].height

	for _, entry := range formattedContents {
		if entry.height == highestHeight {
			tiedEntries = append(tiedEntries, entry)
		} else {
			break
		}
	}

	slices.SortFunc(tiedEntries, func(a, b HeightEntry) int {
		return strings.Compare(a.hash, b.hash)
	})

	return tiedEntries[0].height, tiedEntries[0].hash, nil
}

func (bb *Broombase) ValidateBlockHeaders(block *Block) error {

	if !block.ValidateHash() {
		return errors.New("block error: invalid hash")
	}

	_, found := bb.GetBlock(block.Hash, block.Height)
	if found {
		return errors.New("block error: exact block already exists")
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

	} else {
		if block.Hash != GENESIS_HASH {
			return errors.New("invalid genesis block hash, attemp made to attack genesis block")
		}
	}

	return nil
}

// DOES NOT USE SNAPSHOTS
// ONLY USED ON GENESIS
func (bb *Broombase) AddAndSync(block *Block) error {

	err := bb.StoreBlock(*block)
	if err != nil {
		return err
	}

	// SLOW HERE
	if bb.ledger.BlockHeight < block.Height {
		fmt.Println("syncing ledger")
		err = bb.SyncLedger(block.Height, block.Hash)
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

	startBlockHeight := blockHeight
	startBlockHash := blockHash

	//clear ledger (can't sync with old data)
	bb.ledger.Clear()

	// keep as much data not in memory as possible
	type heightHashTime struct {
		height int64
		hash   string
		time   time.Time
	}

	fingerPrint := make([]heightHashTime, blockHeight+1)

	// trace backwards through chain until genesis
	for blockHeight >= 0 {

		block, found := bb.GetBlock(blockHash, blockHeight)
		if !found {
			return errors.New("SyncLedger: could not find block")
		}
		fingerPrint[blockHeight] = heightHashTime{
			height: block.Height,
			hash:   block.Hash,
			time:   block.GetTimestampTime(),
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

	err := bb.StoreLedger(startBlockHash, startBlockHeight)
	if err != nil {
		fmt.Println("storing ledger in database")
		return err
	}

	return nil
}

func (bb *Broombase) StoreLedger(hash string, height int64) error {

	_, found := bb.GetLedgerAt(hash, height)
	if found {
		fmt.Println("Ledger found, not storing")
		// we already have the ledger
		return nil
	}

	fmt.Println("Storing ledger", height, hash)

	bb.ledger.mut.Lock()
	defer bb.ledger.mut.Unlock()

	data, err := json.Marshal(bb.ledger)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/%d_%s.broomledger", bb.ledgerDir, height, hash)
	return os.WriteFile(path, data, 0644)
}

func (bb *Broombase) StoreGivenLedger(ledger *Ledger) error {

	_, found := bb.GetLedgerAt(ledger.BlockHash, ledger.BlockHeight)
	if found {
		// we already have the ledger
		return nil
	}

	bb.ledger.mut.Lock()
	defer bb.ledger.mut.Unlock()

	fmt.Println("storage lock aquired")

	data, err := json.Marshal(ledger)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/%d_%s.broomledger", bb.ledgerDir, ledger.BlockHeight, ledger.BlockHash)
	return os.WriteFile(path, data, 0644)
}

func (bb *Broombase) GetLedgerAt(hash string, height int64) (*Ledger, bool) {

	bb.ledger.mut.RLock()
	defer bb.ledger.mut.RUnlock()

	path := fmt.Sprintf("%s/%d_%s.broomledger", bb.ledgerDir, height, hash)
	data, err := os.ReadFile(path)
	if err != nil {
		// file not found or read error
		return nil, false
	}

	ledger := &Ledger{}

	err = json.Unmarshal(data, ledger)
	if err != nil {
		return nil, false
	}

	// ledger.mut = &sync.RWMutex{}

	return ledger, true
}

// block must validate against current ledger
// TODO: Validate Mining difficulty
func (l *Ledger) ValidateBlock(block Block) error {

	l.mut.RLock()
	defer l.mut.RUnlock()

	fmt.Println("lock aquired")

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

	fmt.Println("starting moving through txns")

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

	fmt.Println("starting nonce validation")

	for from, txnGroup := range accountTransactions {
		// check nonces and sum

		err := ValidateTransactionGroup(l.Balances[from], l.Nonces[from], txnGroup)
		if err != nil {
			return err
		}
	}

	fmt.Println("done validating block")

	return nil

}

func (l *Ledger) GetAddressNonce(address string) (int64, bool) {
	l.mut.RLock()
	defer l.mut.RUnlock()

	nonce, found := l.Nonces[address]

	return nonce, found

}

func (l *Ledger) GetAddressBalance(address string) (int64, bool) {
	l.mut.RLock()
	defer l.mut.RUnlock()

	balance, found := l.Balances[address]

	return balance, found

}

// other block txns are needed to validate the current txn
// The current "txn to validate" can be added to the list of the rest of the txns,
func ValidateTransactionGroup(currentBalance int64, currentNonce int64, txns []Transaction) error {

	// check sum of txns is valid
	var sendAmount int64
	for _, txn := range txns {
		sendAmount += txn.Amount
	}

	if currentBalance-sendAmount < 0 {
		return errors.New("invalid ")
	}

	// check nonces
	_, err := ValidateNonce(txns, currentNonce)
	if err != nil {
		return err
	}

	return nil

}

// first txn nonce must be 1
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

	l.MiningThresholdHash = l._calculateNewMiningThreshold()

	// loads the block time array in prep for averaging
	l._transitionBlockTimeArray(BlockTimeDifficulty{
		Height:     b.Height,
		Time:       b.GetTimestampTime(),
		Difficulty: l.MiningThresholdHash,
	})

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

func (l *Ledger) _calculateNewMiningThreshold() string {
	if len(l.BlockTimeDifficulty) < 4 {
		fmt.Println("using default threshold")
		return DEFAULT_MINING_THRESHOLD
	}

	tracker := make(map[int64]BlockTimeDifficulty)
	for _, btd := range l.BlockTimeDifficulty {
		if btd.Height != 0 {
			tracker[btd.Height] = btd
		}
	}

	gaps := []int{}
	bigs := []big.Int{}

	for _, diff := range l.BlockTimeDifficulty {

		_, found := tracker[diff.Height-1]
		if found {
			num := new(big.Int)
			num.SetString(diff.Difficulty, 16)

			bigs = append(bigs, *num)

			gap := diff.Time.Sub(tracker[diff.Height-1].Time).Seconds()
			gaps = append(gaps, int(gap))
		}
	}

	sum := 0
	for _, gap := range gaps {
		sum += gap
	}

	bigSum := big.NewInt(0)
	for i := range bigs {
		bigSum.Add(bigSum, &bigs[i])
	}

	average := sum / len(gaps)
	bigAverage := bigSum.Div(bigSum, big.NewInt(int64(len(bigs))))

	if average == TARGET_BLOCK_TIME {
		return fmt.Sprintf("%064x", bigAverage)
	}

	oldDifficultyString := ""
	highestHeight := 0

	for _, d := range l.BlockTimeDifficulty {
		if d.Height > int64(highestHeight) {
			highestHeight = int(d.Height)
			oldDifficultyString = d.Difficulty
		}
	}

	oldDifficulty := new(big.Int)
	oldDifficulty.SetString(oldDifficultyString, 16)

	alpha := big.NewRat(1, ALPHA_FACTOR)

	difference := big.NewInt(int64(average - TARGET_BLOCK_TIME))

	ratio := new(big.Rat).SetFrac(difference, big.NewInt(TARGET_BLOCK_TIME))

	alphaProduct := new(big.Rat).Mul(alpha, ratio)

	one := big.NewRat(1, 1)

	alphaProductP1 := new(big.Rat).Add(one, alphaProduct)

	oldDiffRat := new(big.Rat).SetInt(oldDifficulty) // oldDiff is *big.Int
	newDiffRat := new(big.Rat).Mul(oldDiffRat, alphaProductP1)

	newDiffInt := new(big.Int).Div(newDiffRat.Num(), newDiffRat.Denom())

	return fmt.Sprintf("%064x", newDiffInt)
}

func (l *Ledger) GetCurrentMiningThreshold() string {
	l.mut.RLock()
	defer l.mut.RUnlock()

	thresh := l.MiningThresholdHash

	return thresh
}

func (l *Ledger) CalculateCurrentReward() int {
	return STARTING_PAYOUT
}

func (l *Ledger) Clear() {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.BlockHeight = -1
	l.Balances = make(map[string]int64)
	l.Nonces = make(map[string]int64)
	l.Pending = l.Pending[:0]
	l.BlockTimeDifficulty = make([]BlockTimeDifficulty, 0)
}

// Pass in a previously validated block, use l.ValidateBlock()
// CurrentLedger
func (l *Ledger) SyncNextBlock(validatedBlock Block) {
	// one check to ensure the hashes match
	l.Accumulate(validatedBlock)

}

func (l *Ledger) CheckLedger(address string) (found bool, nonce int64, balance int64) {
	l.mut.RLock()
	defer l.mut.RUnlock()

	nonce, found = l.Nonces[address]

	balance, found = l.Balances[address]

	return
}

// not ledger safe,
func (bb *Broombase) ReceiveBlock(block Block) error {

	err := bb.ValidateBlockHeaders(&block)
	if err != nil {
		return err
	}

	// check if it solves current puzzle
	if bb.ledger.BlockHeight+1 == block.Height && bb.ledger.BlockHash == block.PreviousBlockHash {
		// this is the next block

		err := bb.ledger.ValidateBlock(block)
		if err != nil {
			return err
		}
		// my current ledger should be up to date
		bb.ledger.SyncNextBlock(block)

		err = bb.StoreLedger(block.Hash, block.Height)
		if err != nil {
			panic("cant store ledger but we already synced")
		}

		err = bb.StoreBlock(block)
		if err != nil {
			panic("cant store block but we already synced and stored ledger")
		}

		return nil
	} else {
		// this block is not the correct puzzle target, we must still reconcile for forks
		// get the ledger at the previous block value
		ledger, found := bb.GetLedgerAt(block.PreviousBlockHash, block.Height-1)
		if !found {
			return errors.New("Block has no associated ledger, we do not have previous")
		}

		ledger.mut = &sync.RWMutex{}

		fmt.Println("LEDGER FOUND")
		fmt.Println("ledger: ", ledger)

		err := ledger.ValidateBlock(block)
		if err != nil {
			fmt.Println(err)
			return errors.New("could not validate block of prev value ledger")
		}
		fmt.Println("syncing the block")
		ledger.SyncNextBlock(block)
		fmt.Println("done syncing")
		err = bb.StoreGivenLedger(ledger)
		if err != nil {
			return err
		}
		err = bb.StoreBlock(block)
		if err != nil {
			return err
		}
	}

	return nil
}

// used for testing only and copied by TempStorage
func (bb *Broombase) GetFirstBlockByHeight(height int) (string, bool) {
	bb.mut.RLock()
	files, err := os.ReadDir(bb.dir)
	bb.mut.RUnlock()
	if err != nil {
		return "", false
	}

	for _, file := range files {
		splitName := strings.Split(file.Name(), "_")
		heightValue, _ := strconv.Atoi(splitName[0])
		if heightValue == height {
			hashValue := strings.Split(splitName[1], ".broom")[0]
			return hashValue, true
		}
	}

	return "", false

}

func (l *Ledger) _transitionBlockTimeArray(bt BlockTimeDifficulty) {
	l.BlockTimeDifficulty = append(l.BlockTimeDifficulty, bt)

	if len(l.BlockTimeDifficulty) > BLOCK_SPEED_AVERAGE_WINDOW {
		// find lowest Height
		minIdx := 0
		for i, curBt := range l.BlockTimeDifficulty {
			if curBt.Height < l.BlockTimeDifficulty[minIdx].Height {
				minIdx = i
			}
		}
		// remove lowest Height
		l.BlockTimeDifficulty = append(l.BlockTimeDifficulty[:minIdx], l.BlockTimeDifficulty[minIdx+1:]...)
	}
}

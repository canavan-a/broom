package netnode

import (
	"bytes"
	"fmt"
	"maps"
	"math/big"
	"math/rand"

	"time"

	"encoding/binary"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/canavan-a/broom/node/crypto"
)

const COINBASE = "coinbase"
const MAX_BLOCK_SIZE = 1000
const STARTING_PAYOUT = 10_000
const COINBASE_VESTING_BLOCK_NUMBER = 10 // coinbase txns don't go out until a fork is hopefully resolved, these txn amounts are not spendable for this number of blocks

type Block struct {
	Hash string `json:"hash"`

	Timestamp         int64           `json:"timestamp"`
	Height            int64           `json:"height"`
	Nonce             int64           `json:"nonce"`
	PreviousBlockHash string          `json:"previous"`
	Transactions      TransactionPool `json:"transactions"`
}

type TransactionPool map[string]Transaction

func NewBlock(myAddress string, note string, PreviousBlockHash string, height int64, currentReward int64) *Block {

	// make coinbase Txn
	coinbaseTxn := &Transaction{
		Sig: "", // no sender

		Coinbase: true,
		Note:     note,
		Nonce:    0, // no nonce to increment (could use the block height)
		To:       myAddress,
		From:     "", // from coinbase
		Amount:   currentReward,
	}

	txns := make(map[string]Transaction)

	txns[COINBASE] = *coinbaseTxn

	return &Block{
		PreviousBlockHash: PreviousBlockHash,
		Transactions:      txns,
		Height:            height,
	}

}

func (b *Block) Serialize() []byte {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, b.Timestamp)
	binary.Write(buf, binary.BigEndian, b.Height)
	binary.Write(buf, binary.BigEndian, b.Nonce)
	buf.WriteString(b.PreviousBlockHash)

	var txns []Transaction

	for _, txn := range b.Transactions {
		txns = append(txns, txn)
	}

	slices.SortFunc(txns, func(a, b Transaction) int {
		return strings.Compare(a.Sig, b.Sig)
	})

	for _, txn := range txns {
		buf.Write(txn.Serialize())
	}

	return buf.Bytes()
}

func (b *Block) CalculateHash() string {
	serialized := b.Serialize()

	return crypto.Hash(serialized)
}

func (b *Block) SignHash() {
	b.Hash = b.CalculateHash()
}

func (b *Block) ValidateHash() bool {
	return b.CalculateHash() == b.Hash
}

func (b *Block) SetStampNow() {
	b.Timestamp = time.Now().Unix()
}

func (b *Block) GetTimestampTime() time.Time {
	return time.Unix(b.Timestamp, 0)
}

func (b *Block) RandomizeNonce() {
	b.Nonce = rand.Int63()
}

func (b *Block) Add(t Transaction) {
	b.Transactions[t.Sig] = t
}

func (b *Block) RotateMiningValues() (hash string) {
	b.SetStampNow()
	b.RandomizeNonce()
	b.SignHash()

	return b.Hash
}

func (b Block) StartSolutionWorker(target string, solutionChan chan Block, done chan struct{}) {
	fmt.Println("worker started")
	for {
		select {
		case <-done:
			return
		default:
			b.RotateMiningValues()
			if CompareHash(b.Hash, target) {
				select {
				case solutionChan <- b:
				case <-done:
				}
				return
			}
		}
	}
}

func CompareHash(hash, target string) bool {
	hb, _ := hex.DecodeString(hash)
	tb, _ := hex.DecodeString(target)

	hi := new(big.Int).SetBytes(hb)
	ti := new(big.Int).SetBytes(tb)

	return hi.Cmp(ti) == -1

}

func (b Block) DeepCopy() Block {
	txCopy := make(TransactionPool, len(b.Transactions))
	maps.Copy(txCopy, b.Transactions)

	b.Transactions = txCopy
	return b
}

func (b *Block) MineWithWorkers(target string, workers int, solutionChan chan Block, done chan struct{}) {

	go func() {
		for range workers {
			bCopy := b.DeepCopy()
			go bCopy.StartSolutionWorker(target, solutionChan, done)
		}
	}()
}

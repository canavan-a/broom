package netnode

import (
	"bytes"
	"maps"
	"math/rand"

	"time"

	"encoding/binary"
	"slices"
	"strings"

	"github.com/canavan-a/broom/node/crypto"
)

const MAX_BLOCK_SIZE = 1000

type MemPool struct {
	ValidTransactions TransactionPool
}

type Block struct {
	Hash string `json:"hash"`

	Timestamp         int64           `json:"timestamp"`
	Height            int64           `json:"height"`
	Nonce             int64           `json:"nonce"`
	PreviousBlockHash string          `json:"previous"`
	Transactions      TransactionPool `json:"transactions"`
}

type TransactionPool map[string]Transaction

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

	for {
		select {
		case <-done:
			return
		default:
			b.RotateMiningValues()
			if b.Hash < target {
				select {
				case solutionChan <- b:
				case <-done:
				}
				return
			}
		}
	}
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

type BlockChain struct {
	Prev    Block
	Current Block
}

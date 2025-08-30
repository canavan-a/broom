package netnode

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
)

const MAX_BLOCK_SIZE = 1000

type MemPool struct {
	ValidTransactions TransactionPool
}

type Block struct {
	Hash string `json:"hash"`

	Height            int64           `json:"height"`
	Nonce             int64           `json:"nonce"`
	PreviousBlockHash string          `json:"previous"`
	Transactions      TransactionPool `json:"transactions"`
}

type TransactionPool map[string]Transaction

func (b *Block) Serialize() []byte {

	buf := new(bytes.Buffer)

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

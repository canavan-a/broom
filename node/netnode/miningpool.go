package netnode

import (
	"encoding/gob"
	"fmt"
	"os"
)

const MINING_POOL_GOB = "mining_pool.gob"

// stores proof groups for specific heights
type MiningPool struct {
	TaxPercent     int64
	BlockPayout    int64
	NodeAddress    string
	NodePrivateKey string
	TxnChan        chan Transaction
	WorkLog        map[int64]WorkTrie

	GetCurrentNonce func(address string) int64

	PoolNote string
}

// stores proof groups per address
type WorkTrie struct {
	Members map[string]WorkLeaf
}

// stores proofs and counts
type WorkLeaf struct {
	Count  int64
	Proofs map[string]WorkProof
}

// stores the dedicated proof block
type WorkProof struct {
	Address string `json:"address"`
	Block   Block  `json:"block"`
}

func newMiningPool(
	taxPercent int64,
	blockPayout int64,
	nodeMiningAddress string,
	txnChan chan Transaction,
	poolNote string,
	privateKey string,
	getNonce func(address string) int64,
) *MiningPool {

	// validate the pub and private keys (this will panic if invalid)
	CheckKeys(nodeMiningAddress, privateKey)

	return &MiningPool{
		WorkLog:         make(map[int64]WorkTrie),
		TaxPercent:      taxPercent,
		BlockPayout:     blockPayout,
		NodeAddress:     nodeMiningAddress,
		TxnChan:         txnChan,
		PoolNote:        poolNote,
		NodePrivateKey:  privateKey,
		GetCurrentNonce: getNonce,
	}
}

func InitMiningPool(
	taxPercent int64,
	blockPayout int64,
	nodeMiningAddress string,
	txnChan chan Transaction,
	poolNote string,
	privateKey string,
	getNonce func(address string) int64,
) *MiningPool {
	mp := newMiningPool(taxPercent, blockPayout, nodeMiningAddress, txnChan, poolNote, privateKey, getNonce)

	mp.LoadBackup() // pull in existing mining pool worklog (ignore error)
	// if err != nil {
	// 	fmt.Println("error loading backup")
	// 	panic(err)
	// }

	err := mp.BackupWorkLog()
	if err != nil {
		fmt.Println("erroring here")
		panic(err)
	}

	return mp
}

func (mp *MiningPool) BackupWorkLog() error {
	file, err := os.Create(MINING_POOL_GOB)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := gob.NewEncoder(file)
	enc.Encode(mp.WorkLog)
	return nil
}

func (mp *MiningPool) LoadBackup() error {
	file, err := os.Open(MINING_POOL_GOB)
	if err != nil {
		return err
	}
	defer file.Close()

	dec := gob.NewDecoder(file)
	var decodedMiningPool map[int64]WorkTrie
	dec.Decode(&decodedMiningPool)
	mp.WorkLog = decodedMiningPool

	return nil
}

func CheckKeys(pub, priv string) {
	mockTxn := Transaction{
		Coinbase: false,
		To:       "nobody",
		From:     pub,
		Nonce:    0,
		Note:     "hello",
		Amount:   44444,
	}
	err := mockTxn.Sign(priv)
	if err != nil {
		panic(err)
	}
	valid, err := mockTxn.ValidateSig()
	if err != nil {
		panic(err)
	}
	if !valid {
		panic("invalid sig validation")
	}

}

func (mp *MiningPool) AddWorkProof(address string, block Block) {
	trie, ok := mp.WorkLog[block.Height]
	if !ok {
		trie = WorkTrie{Members: make(map[string]WorkLeaf)}
	}

	leaf, ok := trie.Members[address]
	if !ok {
		leaf = WorkLeaf{Proofs: make(map[string]WorkProof)}
	}

	_, found := leaf.Proofs[block.Hash]
	if !found {
		leaf.Count++
	}

	leaf.Proofs[block.Hash] = WorkProof{Block: block, Address: address}
	trie.Members[address] = leaf
	mp.WorkLog[block.Height] = trie

	mp.BackupWorkLog()
}

func (mp *MiningPool) ClearBlock(i int64) {
	delete(mp.WorkLog, i)
	mp.BackupWorkLog()
}

func (mp *MiningPool) PayoutBlock(i int64) {

	defer mp.ClearBlock(i)

	potPercent := 100 - mp.TaxPercent
	pot := mp.BlockPayout * potPercent / 100

	var totalProofs int64
	for _, member := range mp.WorkLog[i].Members {
		totalProofs += member.Count
	}

	if totalProofs == 0 {
		return
	}

	nonce := mp.GetCurrentNonce(mp.NodeAddress)

	for address, member := range mp.WorkLog[i].Members {
		if address == mp.NodeAddress {
			// don't pay yourself
			continue
		}
		claim := member.Count * pot / totalProofs
		nonce++
		mp.Send(claim, address, nonce)
	}

}

func (mp *MiningPool) Send(amount int64, address string, nonce int64) {

	txn := Transaction{
		Coinbase: false,
		To:       address,
		From:     mp.NodeAddress,
		Amount:   amount,
		Note:     mp.PoolNote,
		Nonce:    nonce,
	}

	// this should never error (checked in CheckKeys)
	txn.Sign(mp.NodePrivateKey)

	mp.TxnChan <- txn

}

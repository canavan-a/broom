package netnode

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const EXECUTOR_WORKER_COUNT = 4

type Executor struct {
	node        *Node
	database    *Broombase
	blockChan   chan Block
	txnChan     chan Transaction
	mempool     map[string]Transaction
	miningBlock *Block

	address string
	note    string

	mux *http.ServeMux
}

func NewExecutor(seeds []string, myAddress string, miningNote string, dir string, ledgerDir string, mock bool) *Executor {

	var node *Node
	if mock {
		node = nil
	} else {
		node = ActivateNode(seeds...)

	}

	bb := InitBroombaseWithDir(dir, ledgerDir)
	blockChan := make(chan Block)
	txnChan := make(chan Transaction)
	mempool := make(map[string]Transaction)

	ex := &Executor{
		node:        node,
		database:    bb,
		blockChan:   blockChan,
		txnChan:     txnChan,
		mempool:     mempool,
		miningBlock: NewBlock(myAddress, miningNote, bb.ledger.BlockHash, bb.ledger.BlockHeight+1, int64(bb.ledger.CalculateCurrentReward())),

		address: myAddress,
		note:    miningNote,
	}
	// ex.SetupHttpServer()

	return ex
}

func (ex *Executor) ResetMiningBlock() {
	ex.miningBlock = NewBlock(ex.address, ex.note, ex.database.ledger.BlockHash, ex.database.ledger.BlockHeight+1, int64(ex.database.ledger.CalculateCurrentReward()))
}

func (ex *Executor) RunNetworkSync() {
	// Get my highest block
	// Request highest block from peers
	// Determine algorithm to sync with network

	// Algorith:
	// See if my block is in peers "main" chain
	// YES -> request each next block (request by previous)
	// NO -> step back each block until satisfied

	// what if multiple blocks are added during this operation; we loop the network sync until it returns out directly (return a bool)

	height, _, err := ex.database.getHighestBlock()
	if err != nil {
		panic(err)
	}

	peerHash, peerHeight, err := ex.node.SamplePeersHighestBlock()
	if err != nil {
		// no pers I guess...
		fmt.Println("No peer responses for highest hash")
		return
	}

	if height == int64(peerHeight) {
		// we are synced to the chain
		return
	}

	// get first block of the loop
	peerMaxBlock, err := ex.node.SamplePeersBlock("block", peerHeight, peerHash)
	if err != nil {
		panic(err)
	}
	// we cant store this securely because we dont have a previous block, we should store these in a temp dir and then move them over and validate them
	fmt.Println(peerMaxBlock)

	prev := peerMaxBlock
	// sync loop
	for {
		prev, err := ex.node.SamplePeersBlock("block", int(prev.Height)-1, prev.PreviousBlockHash)
		if err != nil {
			panic(err)
		}

		fmt.Println(prev)
	}

}

func (ex *Executor) RunMiningLoop() {

	doneChan := make(chan struct{})

	ex.Mine(ex.blockChan, doneChan)
	fmt.Println("mining started")

	for {
		fmt.Println("starting new cycle")
		select {
		case block := <-ex.blockChan:

			currentSolution := ex.database.ledger.BlockHeight+1 == block.Height

			// stop mining, handle block validation and storage, start mining again
			fmt.Println("incoming block (network or self)")
			close(doneChan)
			fmt.Println(block.Height)
			fmt.Println("hash: ", block.Hash)
			fmt.Println("prev: ", block.PreviousBlockHash)
			err := ex.database.ReceiveBlock(block)
			if err != nil {
				fmt.Println("Block invalid: ", err)
			} else {
				// no error
				if currentSolution {
					// TODO: smart clear the mempool because we might have valid txns not included in the block
					ex.ResetMiningBlock()
				}

			}

			doneChan = make(chan struct{})
			ex.Mine(ex.blockChan, doneChan)
		case txn := <-ex.txnChan:
			// stop mining, add txn to block, handle block validation, start mining again
			fmt.Println("incoming network txn")
			close(doneChan)

			// TODO: validate the txn, need to validate the block on a specific ledger level

			sigValid, err := txn.ValidateSig()
			if err != nil {
				fmt.Println("Error validating txn sig, pass: ", err)
			}

			sizeValid := txn.ValidateSize()

			if sigValid && sizeValid {
				//TODO: validate nonce and balance against current ledger
				accountBalance, balanceFound := ex.database.ledger.GetAddressBalance(txn.From)

				accountNonce, nonceFound := ex.database.ledger.GetAddressNonce(txn.From)

				if balanceFound && nonceFound {

					addressTxns := ex.getAddressTransactions(txn.From)

					addressTxns = append(addressTxns, txn)
					// toValidate := append(ex.miningBlock.Transactions... )
					err := ValidateTransactionGroup(accountBalance, accountNonce, addressTxns)
					if err != nil {
						// pass: do nothing, txn is not validated

					} else {
						// we found a good txn, add it to the mempool
						ex.mempool[txn.Sig] = txn
						ex.miningBlock.Add(txn)
					}
				}

			}

			doneChan = make(chan struct{})
			ex.Mine(ex.blockChan, doneChan)

		}

	}

}

func (ex *Executor) Mine(solutionChan chan Block, doneChan chan struct{}) {

	ex.miningBlock.MineWithWorkers(ex.database.ledger.CalculateNewMiningThreshold(), EXECUTOR_WORKER_COUNT, solutionChan, doneChan)
}

func (ex *Executor) getAddressTransactions(address string) []Transaction {

	var addressTxns []Transaction

	for _, txn := range ex.miningBlock.Transactions {
		if txn.From == address {
			addressTxns = append(addressTxns, txn)
		}
	}

	return addressTxns
}

func (ex *Executor) SetupHttpServer() {
	go func() {
		ex.mux = http.NewServeMux()
		ex.server_GetBlock() // actually post still
		ex.server_GetAddressDetails()
		ex.server_PostTransaction()
		ex.server_PostBlock() // receives a block
		ex.server_HighestBlock()

		http.ListenAndServe(fmt.Sprintf(":%s", EXPOSED_PORT), ex.mux)
	}()

}

type HashHeight struct {
	Height int    `json:"height"`
	Hash   string `json:"hash"`
}

func (ex *Executor) server_GetBlock() {
	ex.mux.Handle("/block_get", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var p HashHeight
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		block, found := ex.database.GetBlock(p.Hash, int64(p.Height))
		if !found {
			http.Error(w, "block not found", 404)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(block); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	}))
}

func (ex *Executor) server_GetAddressDetails() {
	ex.mux.Handle("/address", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type Payload struct {
			Address string `json:"address"`
		}
		type Response struct {
			Balance int64 `json:"balance"`
			Nonce   int64 `json:"nonce"`
		}
		var p Payload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		var res Response

		found, nonce, balance := ex.database.ledger.CheckLedger(p.Address)
		if !found {
			http.Error(w, "address details not found", 404)
			return
		}

		res.Balance = balance
		res.Nonce = nonce

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	}))
}

func (ex *Executor) server_PostTransaction() {
	ex.mux.Handle("/transaction", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var txn Transaction
		if err := json.NewDecoder(r.Body).Decode(&txn); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		sizeValidated := txn.ValidateSize()
		if !sizeValidated {
			http.Error(w, "invalid txn size", 400)
			return
		}

		validated, err := txn.ValidateSig()
		if err != nil || !validated {
			http.Error(w, "invalid txn", 400)
			return
		}

		// ingress txn
		ex.txnChan <- txn
		w.Write([]byte("ok"))

	}))
}

func (ex *Executor) server_PostBlock() {
	ex.mux.Handle("/block", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var block Block
		if err := json.NewDecoder(r.Body).Decode(&block); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		// ingress txn
		ex.blockChan <- block
		w.Write([]byte("ok"))

	}))
}

func (ex *Executor) server_HighestBlock() {
	ex.mux.Handle("/highest_block", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var hh HashHeight

		height, hash, err := ex.database.getHighestBlock()
		if err != nil {
			http.Error(w, "highest block not found", 404)
			return
		}

		hh.Hash = hash
		hh.Height = int(height)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(hh); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	}))
}

package netnode

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const EXECUTOR_WORKER_COUNT = 4
const SYNC_CHECK_DURATION = 2 * time.Minute

type Executor struct {
	mining      bool
	controlChan chan struct{}

	node        *Node
	database    *Broombase
	blockChan   chan Block
	txnChan     chan Transaction
	mempool     map[string]Transaction
	miningBlock *Block

	address string
	note    string

	mux *http.ServeMux

	egressBlockChan chan Block
	egressTxnChan   chan Transaction
}

func NewExecutor(myAddress string, miningNote string, dir string, ledgerDir string) *Executor {

	bb := InitBroombaseWithDir(dir, ledgerDir)
	blockChan := make(chan Block)
	txnChan := make(chan Transaction)

	egressBlockChan := make(chan Block)
	egressTxnChan := make(chan Transaction)
	mempool := make(map[string]Transaction)

	ex := &Executor{
		controlChan: make(chan struct{}),
		database:    bb,
		blockChan:   blockChan,
		txnChan:     txnChan,
		mempool:     mempool,
		miningBlock: NewBlock(myAddress, miningNote, bb.ledger.BlockHash, bb.ledger.BlockHeight+1, int64(bb.ledger.CalculateCurrentReward())),

		address: myAddress,
		note:    miningNote,

		egressBlockChan: egressBlockChan,
		egressTxnChan:   egressTxnChan,
	}

	return ex
}

func (ex *Executor) Start(workers int, seeds ...string) {

	ex.node = ActivateNode(ex.blockChan, ex.egressBlockChan, ex.txnChan, ex.egressTxnChan, seeds...)

	if len(seeds) != 0 {
		for {
			if len(ex.node.GetAddressSample()) != 0 {
				break
			}
			time.Sleep(1 * time.Second)
			fmt.Println("waiting for at least one peer")
		}
	}

	fmt.Println("Starting rest server")
	ex.SetupHttpServer()

	ctx, cancel := context.WithCancel(context.Background())

	fmt.Println("Syncing to network")
	ex.NetworkSync(ctx)

	fmt.Println("Node running")
	ex.RunMiningLoop(ctx, workers)

	<-ex.controlChan
	cancel()

}

func (ex *Executor) Cancel() {
	ex.controlChan <- struct{}{}
}

func (ex *Executor) ResetMiningBlock() {

	// try to mine off highest block

	height, hash, err := ex.database.getHighestBlock()
	if err != nil {
		panic("could not find highest block")
	}

	ledger, found := ex.database.GetLedgerAt(hash, height)
	if !found {
		panic("no ledger found for highest block")
	}

	ledger.mut = &sync.RWMutex{}

	ex.database.ledger = ledger

	ex.miningBlock = NewBlock(ex.address, ex.note, hash, height+1, int64(ex.database.ledger.CalculateCurrentReward()))
}

func (ex *Executor) NetworkSync(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		caughtUp := ex.RunNetworkSync(ctx)
		if caughtUp {
			fmt.Println("caught up to network")
			break
		}
	}
}

func (ex *Executor) NetworkSyncWithTracker(ctx context.Context) (syncRequired bool) {

	for {
		if ctx.Err() != nil {
			return
		}
		caughtUp := ex.RunNetworkSync(ctx)
		if caughtUp {
			break
		}
		syncRequired = true

	}

	return
}

func (ex *Executor) RunNetworkSync(ctx context.Context) (caughtUp bool) {
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
		fmt.Println("No peer responses for highest hash, going solo")
		return true
	}

	if height == int64(peerHeight) {
		// we are synced to the chain
		fmt.Println("peer heights match ")
		return true
	}

	ts := NewTempStorage("")

	// get first block of the loop
	peerMaxBlock, err := ex.node.RacePeersForValidBlock(peerHash, peerHeight)
	if err != nil {
		panic(err)
	}
	// we cant store this securely because we dont have a previous block, we should store these in a temp dir and then move them over and validate them
	fmt.Println(peerMaxBlock)

	err = ts.StoreBlock(peerMaxBlock)
	if err != nil {
		panic(err)
	}

	prev := peerMaxBlock
	// sync loop
	for {

		if ctx.Err() != nil {
			return
		}

		// do we have previous block?
		_, found := ex.database.GetBlock(prev.PreviousBlockHash, prev.Height-1)
		if found {
			fmt.Println("block already exists in broombase")
			break
		}

		// get previous block from network
		prev, err = ex.node.RacePeersForValidBlock(prev.PreviousBlockHash, int(prev.Height)-1)
		if err != nil {
			panic(err)
		}

		ts.StoreBlock(prev)

		fmt.Println(prev)
	}

	ledger, found := ex.database.GetLedgerAt(prev.PreviousBlockHash, prev.Height-1)
	if !found {
		panic("ledger should exist")
		// err = ex.database.SyncLedger(prev.Height, prev.Hash)
		// if err != nil {
		// 	fmt.Println("error on sync", err)
		// }
	} else {
		fmt.Println("setting ledger")
		ex.database.ledger = ledger
		ex.database.ledger.mut = &sync.RWMutex{}
	}

	// move temp blocks over

	lowestHeightToSync := prev.Height
	for {
		if ctx.Err() != nil {
			return
		}
		lowestHash, found := ts.GetFirstBlockByHeight(int(lowestHeightToSync))
		if !found {
			break
		}
		fmt.Println("first block by height", lowestHash)

		lowestBlock, bfound := ts.GetBlock(lowestHash, lowestHeightToSync)
		if !bfound {
			panic("cooked")
		}

		err = ex.database.ReceiveBlock(lowestBlock)
		if err != nil {
			panic(err)
		}
		lowestHeightToSync++

	}

	return

}

func (ex *Executor) RunMiningLoop(ctx context.Context, workers int) {

	ex.mining = true
	defer func() { ex.mining = false }()

	doneChan := make(chan struct{})

	ex.Mine(ctx, ex.blockChan, doneChan, workers)
	fmt.Println("mining started")

	// every 10 min without a block check up and sync
	timer := time.NewTimer(SYNC_CHECK_DURATION)
	defer timer.Stop()

	for {

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			close(doneChan)
			// run a network sync every 5 ish minutes
			fmt.Println("running network sync")
			syncRequired := ex.NetworkSyncWithTracker(ctx)
			if syncRequired {
				//clear the mempool, we had to sync and don't know what txns are added or not added
				ex.mempool = make(map[string]Transaction)
			}

			timer.Reset(SYNC_CHECK_DURATION)
			doneChan = make(chan struct{})

			fmt.Println("sync done")
			ex.Mine(ctx, ex.blockChan, doneChan, workers)

			// send dead block to start mining again
		case block := <-ex.blockChan:

			currentSolution := ex.database.ledger.BlockHeight+1 == block.Height && ex.database.ledger.BlockHash == block.PreviousBlockHash

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

				// share the block with the egress
				ex.egressBlockChan <- block

				// no error
				if currentSolution {
					// smart clear the mempool because we might have valid txns not included in the block
					for txnSig := range block.Transactions {
						delete(ex.mempool, txnSig)
					}
					ex.ResetMiningBlock()

					// copy remaining txns in the mempool into the new block
					maps.Copy(ex.miningBlock.Transactions, ex.mempool)
				}

			}

			doneChan = make(chan struct{})
			ex.Mine(ctx, ex.blockChan, doneChan, workers)
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
						// egress the txn
						ex.egressTxnChan <- txn

						// we found a good txn, add it to the mempool
						ex.mempool[txn.Sig] = txn
						ex.miningBlock.Add(txn)
					}
				}

			}

			doneChan = make(chan struct{})
			ex.Mine(ctx, ex.blockChan, doneChan, workers)

		}

	}

}

func (ex *Executor) Mine(ctx context.Context, solutionChan chan Block, doneChan chan struct{}, workers int) {

	ex.miningBlock.MineWithWorkers(ctx, ex.database.ledger.MiningThresholdHash, workers, solutionChan, doneChan)
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
	ex.mux = http.NewServeMux()
	ex.server_Root()
	ex.server_Difficulty()
	ex.server_GetBlock()
	ex.server_GetAddressDetails()
	ex.server_PostTransaction()
	ex.server_PostBlock()
	ex.server_HighestBlock()

	go func() {
		fmt.Println("Starting HTTP server on port", EXPOSED_PORT)
		if err := http.ListenAndServe(":"+EXPOSED_PORT, ex.mux); err != nil {
			fmt.Println("HTTP server failed:", err)
		}
	}()
}

func (ex *Executor) server_Root() {
	ex.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("I am a node. Please add me to your seed list :)"))
	}))
}

func (ex *Executor) server_Difficulty() {
	thresh := ex.database.ledger.GetCurrentMiningThreshold()
	ex.mux.Handle("/difficulty", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(thresh))
	}))
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

type TempStorage struct {
	base Broombase
}

func NewTempStorage(dir string) *TempStorage {
	if dir == "" {
		dir = "broomtemp"
	}

	err := os.RemoveAll(dir)
	if err != nil {
		fmt.Println("could not remove dir")
	}

	if os.IsNotExist(err) {
		fmt.Println("does not exist, safe to ignore")
	} else if err != nil {
		panic(err)
	}

	err = os.Mkdir(dir, 0755)
	if err != nil {
		panic(err)
	}

	return &TempStorage{base: Broombase{
		mut: sync.RWMutex{},
		dir: dir,
	}}

}

func (ts *TempStorage) StoreBlock(block Block) error {
	return ts.base.StoreBlock(block)
}

func (ts *TempStorage) GetBlock(hash string, height int64) (Block, bool) {
	return ts.base.GetBlock(hash, height)
}

func (ts *TempStorage) DeleteBlock(hash string, height int64) error {
	ts.base.mut.Lock()
	defer ts.base.mut.Unlock()

	err := os.Remove(fmt.Sprintf("%s/%d_%s.broom", ts.base.dir, height, hash))
	if err != nil {
		return err
	}

	return nil
}

func (ts *TempStorage) GetFirstBlockByHeight(height int) (string, bool) {
	return ts.base.GetFirstBlockByHeight(height)
}

func (ex *Executor) RunBackup() error {
	outFile, err := os.Create("backup.tar.gz")
	if err != nil {
		return err
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	dirs := []string{
		ex.database.ledgerDir,
		ex.database.dir,
	}

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			header, err := tar.FileInfoHeader(fi, "")
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(filepath.Dir(dir), file)
			if err != nil {
				return err
			}
			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if fi.Mode().IsRegular() {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err := io.Copy(tw, f); err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

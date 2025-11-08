package netnode

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const EXECUTOR_WORKER_COUNT = 4
const SYNC_CHECK_DURATION = 2 * time.Minute

const CHANNEL_BUFFER_SIZE = 10

const BACKUP_DIR = "backup"

const BACKUP_FREQUENCY = 2 * 60 * 60 // every 2 hours

type Executor struct {
	version string

	mining      bool
	controlChan chan struct{}

	Node     *Node
	Database *Broombase

	MsgChan chan []byte

	BlockChan        chan Block
	TxnChan          chan Transaction
	Mempool          map[string]Transaction
	MiningBlock      *Block
	MiningBlockMutex *sync.RWMutex

	address string
	note    string

	mux *http.ServeMux

	EgressBlockChan chan Block
	EgressTxnChan   chan Transaction

	Port string

	// Mining Pool Config
	MiningPoolEnabled bool
	PoolTaxPercent    int64 // ex: 8 = %8
	PoolNote          string
	PrivateKey        string
	Pool              *MiningPool

	SolutionTargetOperators map[string]func(b Block)
}

func NewExecutor(myAddress string, miningNote string, dir string, ledgerDir string, version string) *Executor {

	bb := InitBroombaseWithDir(dir, ledgerDir)
	blockChan := make(chan Block, CHANNEL_BUFFER_SIZE)
	txnChan := make(chan Transaction, CHANNEL_BUFFER_SIZE)

	egressBlockChan := make(chan Block, CHANNEL_BUFFER_SIZE)
	egressTxnChan := make(chan Transaction, CHANNEL_BUFFER_SIZE)
	mempool := make(map[string]Transaction)

	msgChannel := make(chan []byte, CHANNEL_BUFFER_SIZE)

	// set up solution targets

	soultionTargets := make(map[string]func(b Block))

	soultionTargets[THRESHOLD_A] = func(b Block) {
		fmt.Println("\033[31mTARGET A:\033[0m", b.Hash)
	}

	soultionTargets[THRESHOLD_B] = func(b Block) {
		fmt.Println("\033[32mTARGET B:\033[0m", b.Hash)
	}

	soultionTargets[THRESHOLD_C] = func(b Block) {
		fmt.Println("\033[33mTARGET C:\033[0m", b.Hash)
	}

	ex := &Executor{
		version: version,

		controlChan: make(chan struct{}),
		Database:    bb,

		MsgChan: msgChannel,

		BlockChan:        blockChan,
		TxnChan:          txnChan,
		Mempool:          mempool,
		MiningBlock:      NewBlock(myAddress, miningNote, bb.Ledger.BlockHash, bb.Ledger.BlockHeight+1, int64(bb.Ledger.CalculateCurrentReward())),
		MiningBlockMutex: new(sync.RWMutex),

		address: myAddress,
		note:    miningNote,

		EgressBlockChan: egressBlockChan,
		EgressTxnChan:   egressTxnChan,

		SolutionTargetOperators: soultionTargets,
	}

	return ex
}

func (ex *Executor) SetPort(port string) {

	if port == "" {
		ex.Port = EXPOSED_PORT
	}

	_, err := strconv.Atoi(port)
	if err != nil {
		panic(err)
	}

	ex.Port = port
}

// call this from the cli before running the node
func (ex *Executor) EnableMiningPool(tax int64, privateKey string, poolNote string) {
	ex.MiningPoolEnabled = true
	ex.PoolTaxPercent = tax
	ex.PrivateKey = privateKey
	ex.PoolNote = poolNote

	ex.Pool = InitMiningPool(ex.PoolTaxPercent, STARTING_PAYOUT, ex.address, ex.TxnChan, ex.PoolNote, ex.PrivateKey, ex.GetNodeNonce)

	ex.SolutionTargetOperators[THRESHOLD_POOL] = func(b Block) {
		ex.Pool.PublishWorkProof(WorkProof{Address: ex.address, Block: b})
	}
}

func (ex *Executor) GetNodeNonce(address string) int64 {
	found, nonce, _ := ex.Database.Ledger.CheckLedger(ex.address)
	if !found {
		panic("node nonce must exist, otherwise we can't pay out")
	}
	return nonce

}

func (ex *Executor) AtomicCopyMiningBlock() Block {

	ex.MiningBlockMutex.RLock()
	block := ex.MiningBlock.DeepCopy()
	ex.MiningBlockMutex.RUnlock()

	return block

}

func (ex *Executor) Start(workers int, self string, seeds ...string) {

	ex.Node = ActivateNode(ex.MsgChan, ex.BlockChan, ex.EgressBlockChan, ex.TxnChan, ex.EgressTxnChan, self, seeds...)

	if len(seeds) != 0 {
		for {
			if len(ex.Node.GetAddressSample()) != 0 {
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

	// set backup schedule
	ex.Node.Schedule(func() {
		fmt.Println("runnning backup")
		ex.RunBackup()
		fmt.Println("backup done")
	}, time.Second*BACKUP_FREQUENCY)

	fmt.Println("escaped network sync height", ex.Database.Ledger.BlockHeight)

	ex.MiningBlock.Height = ex.Database.Ledger.BlockHeight + 1

	fmt.Println("Node running")
	ex.RunMiningLoop(ctx, workers)

	<-ex.controlChan
	cancel()

}

func (ex *Executor) Cancel() {
	ex.controlChan <- struct{}{}
}

func (ex *Executor) _resetMiningBlock() {

	// try to mine off highest block

	// height, hash, err := ex.database.getHighestBlock()
	// if err != nil {
	// 	panic("could not find highest block")
	// }

	// ledger, found := ex.database.GetLedgerAt(hash, height)
	// if !found {
	// 	panic("no ledger found for highest block")
	// }

	// ledger.mut = &sync.RWMutex{}

	// ex.database.ledger = ledger

	fmt.Println("Resetting mining block")
	fmt.Println("block hash: ", ex.MiningBlock.Hash)

	ex.MiningBlock = NewBlock(ex.address, ex.note, ex.Database.Ledger.BlockHash, ex.Database.Ledger.BlockHeight+1, int64(ex.Database.Ledger.CalculateCurrentReward()))
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

	height, _, err := ex.Database.GetHighestBlock()
	if err != nil {
		panic(err)
	}

	peerHash, peerHeight, err := ex.Node.SamplePeersHighestBlock()
	if err != nil {
		// no pers I guess...
		fmt.Println("No peer responses for highest hash, going solo")
		return true
	}

	if height == int64(peerHeight) {
		// we are synced to the chain
		return true
	} else if height >= int64(peerHeight) {
		fmt.Println("ahead of peers ")
		return true
	}

	ts := NewTempStorage("")

	// get first block of the loop
	peerMaxBlock, err := ex.Node.RacePeersForValidBlock(peerHash, peerHeight)
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
		_, found := ex.Database.GetBlock(prev.PreviousBlockHash, prev.Height-1)
		if found {
			fmt.Println("block already exists in broombase")
			break
		}

		// get previous block from network
		tempPrev, err := ex.Node.RacePeersForValidBlock(prev.PreviousBlockHash, int(prev.Height)-1)
		if err != nil {
			panic(err)
		}

		// headers will never validate
		prev = tempPrev

		err = ts.StoreBlock(prev)
		if err != nil {
			fmt.Println("ERROR STORING BLOCK:")
			fmt.Println(err)
		}

		fmt.Println(prev)
	}

	ledger, found := ex.Database.GetLedgerAt(prev.PreviousBlockHash, prev.Height-1)
	if !found {
		panic("ledger should exist")
		// err = ex.database.SyncLedger(prev.Height, prev.Hash)
		// if err != nil {
		// 	fmt.Println("error on sync", err)
		// }
	} else {
		fmt.Println("setting ledger")
		ex.Database.Ledger = ledger
		ex.Database.Ledger.Mut = &sync.RWMutex{}
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

		err = ex.Database.ReceiveBlock(lowestBlock)
		if err != nil {
			panic(err)
		}

		fmt.Println("ledger height: ", ex.Database.Ledger.BlockHeight)

		lowestHeightToSync++

	}

	fmt.Println("final sync ledger height: ", ex.Database.Ledger.BlockHeight)

	return

}

func (ex *Executor) RunMiningLoop(ctx context.Context, workers int) {

	ex.mining = true
	defer func() { ex.mining = false }()

	doneChan := make(chan struct{})

	ex.Mine(ctx, ex.BlockChan, doneChan, workers)
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

				ex.MiningBlock.Height = ex.Database.Ledger.BlockHeight + 1
				//clear the mempool, we had to sync and don't know what txns are added or not added
				ex.Mempool = make(map[string]Transaction)
			} else {
				// I may be tracking a higher fork but not using it
				fmt.Println("checking for fork disparity")
				height, hash, err := ex.Database.GetHighestBlock()
				if err != nil {
					panic("must be able to get highets block")
				}

				if height > ex.Database.Ledger.BlockHeight {
					fmt.Println("fixing height disparity")
					ledger, found := ex.Database.GetLedgerAt(hash, height)
					if found {
						ex.Database.Ledger.Mut.Lock()
						ledger.Mut = ex.Database.Ledger.Mut
						fmt.Println("setting ledger")
						ex.Database.Ledger = ledger
						ex.Database.Ledger.Mut.Unlock()

						// clear our mempool
						ex.Mempool = make(map[string]Transaction)
						// set out mining block height
						ex.MiningBlockMutex.Lock()
						ex.MiningBlock.Height = ex.Database.Ledger.BlockHeight + 1
						ex.MiningBlockMutex.Unlock()

					}

				}

			}

			timer.Reset(SYNC_CHECK_DURATION)
			doneChan = make(chan struct{})

			fmt.Println("sync done")
			ex.Mine(ctx, ex.BlockChan, doneChan, workers)

			// send dead block to start mining again
		case block := <-ex.BlockChan:

			currentSolution := ex.Database.Ledger.BlockHeight+1 == block.Height && ex.Database.Ledger.BlockHash == block.PreviousBlockHash

			// stop mining, handle block validation and storage, start mining again
			fmt.Println("incoming block (network or self)")
			close(doneChan)
			fmt.Println(block.Height)
			fmt.Println("hash: ", block.Hash)
			fmt.Println("prev: ", block.PreviousBlockHash)
			err := ex.Database.ReceiveBlock(block)
			if err != nil {
				fmt.Println("Block invalid: ", err)

			} else {

				// share the block with the egress
				ex.EgressBlockChan <- block

				// no error
				if currentSolution {
					fmt.Println("block is current ledger solution")
					// smart clear the mempool because we might have valid txns not included in the block
					for txnSig := range block.Transactions {
						delete(ex.Mempool, txnSig)
					}
					// if this skips and syncs forward we may have txns we try to put in that are already in the chain

					ex.MiningBlockMutex.Lock()
					ex._resetMiningBlock()
					// copy remaining txns in the mempool into the new block
					maps.Copy(ex.MiningBlock.Transactions, ex.Mempool)
					ex.MiningBlockMutex.Unlock()

					// pay out pool members n blocks ago
					if ex.MiningPoolEnabled {
						ex.PayoutMiners(block)
					}
				}

			}

			doneChan = make(chan struct{})
			ex.Mine(ctx, ex.BlockChan, doneChan, workers)
		case txn := <-ex.TxnChan:
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

				fmt.Println("txn from", txn.From)
				accountBalance, balanceFound := ex.Database.Ledger.GetAddressBalance(txn.From)
				fmt.Println("account balance", accountBalance)

				accountNonce, _ := ex.Database.Ledger.GetAddressNonce(txn.From)
				fmt.Println("txn nonce:", txn.Nonce)

				if balanceFound {
					fmt.Println("Found balance and nonce")

					addressTxns := ex.GetAddressTransactions(txn.From)

					addressTxns = append(addressTxns, txn)
					// toValidate := append(ex.miningBlock.Transactions... )
					err := ValidateTransactionGroup(accountBalance, accountNonce, addressTxns)
					if err != nil {
						// pass: do nothing, txn is not validated
						fmt.Println("could not validate txn: ", err)
					} else {
						fmt.Println("adding txn to block, mempool, and sending out")
						// egress the txn
						ex.EgressTxnChan <- txn

						// we found a good txn, add it to the mempool
						ex.MiningBlockMutex.Lock()
						ex.Mempool[txn.Sig] = txn
						ex.MiningBlock._add(txn)
						ex.MiningBlockMutex.Unlock()
					}
				} else {
					fmt.Println("could not find balance and nonce")
				}

			}

			doneChan = make(chan struct{})
			ex.Mine(ctx, ex.BlockChan, doneChan, workers)

		}

	}

}

func (ex *Executor) PayoutMiners(block Block) {
	// 1. trace chain to the desired block height off the correct hash
	// 2. check if you're the block winner
	// 3. trigger pool payout at desired height
	target := block.Height - MINING_PAYOUT_LOOKBACK

	if target <= 0 {
		// highly unlikely pooling starts on early blocks but good safety check
		return
	}

	prev, found := ex.Database.GetBlock(block.PreviousBlockHash, block.Height-1)
	if !found {
		// highly improbable, this block was captured and stored
		panic("no block found on search")
	}

	for {
		prev, found = ex.Database.GetBlock(prev.PreviousBlockHash, prev.Height-1)
		if !found {
			panic("no block found on search")
		}
		if prev.Height == target {
			break
		}
	}

	txn, found := prev._getCoinbaseTransaction()
	if !found {
		fmt.Println("NO Coinbase txn found")
		// no coinbase txn, no block miner payout (highly unlikely)
		return
	}

	if txn.To != ex.address {
		fmt.Println("payout block was not mined by this node")
		// fairly common, block was not mined by this node
		return
	}

	ex.Pool.PayoutBlock(prev.Height)

}

func (ex *Executor) Mine(ctx context.Context, solutionChan chan Block, doneChan chan struct{}, workers int) {
	ex.MiningBlockMutex.RLock()
	ex.MiningBlock.MineWithWorkers(ctx, ex.Database.Ledger.MiningThresholdHash, workers, solutionChan, doneChan, ex.SolutionTargetOperators)
	ex.MiningBlockMutex.RUnlock()
}

func (ex *Executor) GetAddressTransactions(address string) []Transaction {

	var addressTxns []Transaction

	for _, txn := range ex.MiningBlock.Transactions {
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
	ex.server_Msg()
	ex.server_Backup()
	ex.server_Version()

	// only exposed on mining pool enabled nodes
	if ex.MiningPoolEnabled {
		ex.server_PoolEnabled()
		ex.server_Proof()
		ex.server_MiningBlock()
		ex.server_ProofTarget()
	}

	handler := corsMiddleware(ex.mux)

	go func() {

		port := ex.Port
		if port == "" {
			port = EXPOSED_PORT
		}

		fmt.Println("Starting HTTP server on port", port)
		if err := http.ListenAndServe(":"+port, handler); err != nil {
			fmt.Println("HTTP server failed:", err)
		}
	}()
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (ex *Executor) server_Version() {
	ex.mux.Handle("/version", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(ex.version))
	}))
}

func (ex *Executor) server_Root() {
	ex.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("I am a node. Please add me to your seed list :)"))
	}))
}

func (ex *Executor) server_Difficulty() {
	ex.mux.Handle("/difficulty", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		thresh := ex.Database.Ledger.GetCurrentMiningThreshold()
		w.Write([]byte(thresh))
	}))
}

func (ex *Executor) server_PoolEnabled() {
	ex.mux.Handle("/pool", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		b = fmt.Appendf(b, "Mining pool is enabled, the tax rate is %d%% per won block", ex.PoolTaxPercent)
		w.Write(b)
	}))
}

func (ex *Executor) server_ProofTarget() {
	ex.mux.Handle("/proof_target", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Write([]byte(THRESHOLD_POOL))
	}))
}

func (ex *Executor) server_MiningBlock() {
	ex.mux.Handle("/mining_block", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		block := ex.AtomicCopyMiningBlock()

		data, err := json.Marshal(block)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		w.Write(data)
	}))
}

func (ex *Executor) server_Proof() {
	ex.mux.Handle("/proof", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write([]byte("publish pool proofs to this endpoint"))
			return
		}

		var workProof WorkProof
		if err := json.NewDecoder(r.Body).Decode(&workProof); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		// ex.MiningBlock
		err := ex.ValidateIncomingPoolBlock(workProof.Block)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		ex.Pool.PublishWorkProof(workProof)

		w.Write([]byte("proof valid"))
	}))
}

func (ex *Executor) ValidateIncomingPoolBlock(block Block) error {
	// ex.Database.Ledger.
	miningBlock := ex.AtomicCopyMiningBlock()

	// must prove that the miner is mining at the same level so it doesn't skip payouts
	if block.Height != miningBlock.Height {
		return errors.New("invalid block height")
	}

	if block.PreviousBlockHash != miningBlock.PreviousBlockHash {
		return errors.New("invalid previous hash value")
	}

	count := 0
	var coinbaseTxn Transaction
	for _, txn := range block.Transactions {
		if txn.Coinbase {
			count++
			coinbaseTxn = txn
		} else {
			valid, err := txn.ValidateSig()
			if err != nil {
				return errors.New("block contains invalid sig txn")
			}
			if !valid {
				return errors.New("block contains invalid sig txn")
			}

		}
		if count > 1 {
			return errors.New("too many coinbase txns")
		}
	}

	if count == 0 {
		return errors.New("no coinbase txn included")
	}

	if coinbaseTxn.To != ex.address {
		return errors.New("invalid coinbase txn address")
	}

	if coinbaseTxn.Amount != STARTING_PAYOUT {
		return errors.New("invalid coinbase txn payout amount")
	}

	// finally check block hash

	if !block.ValidateHash() {
		return errors.New("invalid block hash")
	}

	return nil

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

		block, found := ex.Database.GetBlock(p.Hash, int64(p.Height))
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

		found, nonce, balance := ex.Database.Ledger.CheckLedger(p.Address)
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

		fmt.Println("Txn headers validated")

		// ingress txn
		ex.TxnChan <- txn
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
		ex.BlockChan <- block
		w.Write([]byte("ok"))

	}))
}

func (ex *Executor) server_HighestBlock() {
	ex.mux.Handle("/highest_block", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var hh HashHeight

		height, hash, err := ex.Database.GetHighestBlock()
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

func (ex *Executor) server_Msg() {
	ex.mux.Handle("/msg", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		msg, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad msg", 400)
			return
		}
		ex.MsgChan <- msg

		w.Write([]byte("Thank you for the msg!"))

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

	dir := BACKUP_DIR

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755) // create if missing
	}

	existingBackupFile := fmt.Sprintf("%s/backup.tar.gz", dir)
	oldBackupFilename := fmt.Sprintf("%s/backup_old.tar.gz", dir)

	_, err := os.Stat(existingBackupFile)
	if err == nil {
		// file exists
		os.Remove(oldBackupFilename)
		os.Rename(existingBackupFile, oldBackupFilename)

	}

	outFile, err := os.Create("backup/backup.tar.gz")
	if err != nil {
		return err
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()
	ex.Database.mut.Lock()
	defer ex.Database.mut.Unlock()
	dirs := []string{
		ex.Database.ledgerDir,
		ex.Database.dir,
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

func (ex *Executor) server_Backup() {
	ex.mux.Handle("/backup", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file := BACKUP_DIR + "/backup.tar.gz"

		if _, err := os.Stat(file); os.IsNotExist(err) {
			http.Error(w, "Backup file not found", 404)
			return
		}

		http.ServeFile(w, r, file)
	}))
}

func (ex *Executor) DownloadBackupFileFromPeer(peerAddress string) {
	secureRequest := ""
	if net.ParseIP(peerAddress) == nil {
		secureRequest = "s"
	}

	resp, err := http.Get(fmt.Sprintf("http%s://%s/backup", secureRequest, peerAddress))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		panic("invalid status code")
	}

	out, err := os.Create("backup.tar.gz")
	if err != nil {
		panic(err)
	}
	defer out.Close()

	var written int64
	progress := func(p int64) {
		fmt.Printf("\rDownloaded %d / %d bytes", p, resp.ContentLength)

	}

	reader := io.TeeReader(resp.Body, out)
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			written += int64(n)
			progress(written)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("\nDone.")
}

func (ex *Executor) BackupFromFile(filename string) {
	os.RemoveAll(BROOMBASE_DEFAULT_DIR)
	os.RemoveAll(LEDGER_DEFAULT_DIR)

	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		panic(err)
	}
	defer gz.Close()

	tarReader := tar.NewReader(gz)

	// first pass: count + total size
	var fileCount int
	var totalSize int64
	for {
		h, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		if h.Typeflag == tar.TypeReg {
			fileCount++
			totalSize += h.Size
		}
	}

	// rewind
	f.Seek(0, io.SeekStart)
	gz, _ = gzip.NewReader(f)
	tarReader = tar.NewReader(gz)

	// extract with progress
	var extracted int
	var extractedSize int64
	for {
		h, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(h.Name, 0755)
		case tar.TypeReg:
			outFile, _ := os.Create(h.Name)
			n, _ := io.Copy(outFile, tarReader)
			outFile.Close()

			extracted++
			extractedSize += n
			percent := float64(extractedSize) / float64(totalSize) * 100
			fmt.Printf("\rExtracted %d/%d files (%.1f%%)", extracted, fileCount, percent)
		}
	}
	fmt.Println("\nDone.")
}

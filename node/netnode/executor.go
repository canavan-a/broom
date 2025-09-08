package netnode

import "fmt"

type Executor struct {
	node        *Node
	database    *Broombase
	blockChan   chan Block
	txnChan     chan Transaction
	mempool     map[string]Transaction
	miningBlock *Block

	address string
	note    string
}

func NewExecutor(seeds []string, myAddress string, miningNote string, mock bool) *Executor {

	var node *Node
	if mock {
		node = nil
	} else {
		node = ActivateNode(seeds...)

	}

	bb := NewBroombase()
	blockChan := make(chan Block)
	txnChan := make(chan Transaction)
	mempool := make(map[string]Transaction)

	return &Executor{
		node:        node,
		database:    bb,
		blockChan:   blockChan,
		txnChan:     txnChan,
		mempool:     mempool,
		miningBlock: NewBlock(myAddress, miningNote, bb.ledger.BlockHash, bb.ledger.BlockHeight+1, int64(bb.ledger.CalculateCurrentReward())),

		address: myAddress,
		note:    miningNote,
	}
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

	ex.miningBlock.MineWithWorkers(ex.database.ledger.CalculateNewMiningThreshold(), 2, solutionChan, doneChan)
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

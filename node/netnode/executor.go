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

func NewExecutor(seeds []string, myAddress string, miningNote string) *Executor {
	bb := NewBroombase()
	node := ActivateNode(seeds...)
	blockChan := make(chan Block)
	txnChan := make(chan Transaction)
	mempool := make(map[string]Transaction)

	return &Executor{
		node:        node,
		database:    bb,
		blockChan:   blockChan,
		txnChan:     txnChan,
		mempool:     mempool,
		miningBlock: NewBlock(myAddress, miningNote, bb.ledger.BlockHash, bb.ledger.BlockHeight, int64(bb.ledger.CalculateCurrentReward())),

		address: myAddress,
		note:    miningNote,
	}
}

func (ex *Executor) ResetMiningBlock() {
	ex.miningBlock = NewBlock(ex.address, ex.note, ex.database.ledger.BlockHash, ex.database.ledger.BlockHeight, int64(ex.database.ledger.CalculateCurrentReward()))
}

func (ex *Executor) RunLoop() {

	doneChan := make(chan struct{})

	ex.Mine(ex.blockChan, doneChan)

	for {

		select {
		case block := <-ex.blockChan:

			currentSolution := ex.database.ledger.BlockHeight+1 == block.Height

			// stop mining, handle block validation and storage, start mining again
			fmt.Println("incoming network block")
			close(doneChan)
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

			//TODO: validate nonce and balance against current ledger

			if sigValid && sizeValid {
				// ex.database.ledger.
				ex.mempool[txn.Sig] = txn
				ex.miningBlock.Add(txn)
			}

			doneChan = make(chan struct{})
			ex.Mine(ex.blockChan, doneChan)

		}

	}

}

func (ex *Executor) Mine(solutionChan chan Block, doneChan chan struct{}) {

	ex.miningBlock.MineWithWorkers(ex.database.ledger.CalculateNewMiningThreshold(), 10, solutionChan, doneChan)
}

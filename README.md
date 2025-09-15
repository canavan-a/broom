# Broom

A distributed ledger written in Go based loosely on the Satoshi Nakamotot bitcoin whitepaper. Credit also given to Gary Gensler's course on MIT openCourseWare: Blockchain and Money. `Zero` dependencies outside of the Go standard library.

## Background

Broom is not a token. It is the ledger and blockchain infrastructure that will support a future, unamed token. The name comes from history: in Medieval England, tally sticks were used to record debts by carving marks into wood. A broom, being a bundle of sticks, represents strength and resilience through unity. In Broom, each node is like a stick, individually small but collectively forming a secure, reliable system. Even if one fails, the whole network remains effective.

## Overview

The ledger is backed by Ed25519 Elliptic curve key signiatures for all cryptography. Addresses are generated from the public key and Kekccak256 hash. Broom uses JSON messages over TCP to transmit information between nodes. The starting verification algorithm is Proof of Work using a nunce formula. Proof of Stake will be implemented at a later date.

## Node messaging protocol

Nodes communicate on port 4188 and messages are passed via a bytestream of encoded json. Each message starts with a unique byte delimeter 276

protocol looks like:

`276` `8 bytes length value` `data (however long the length value says)`

## Cryptography

### PoW: Argon2d

The hash function used in mining as well as transaction signing is a Argon2d. This is a variant of Argon2 that is memory hardened. The goal is to resist against ASIC and GPU mining. Argon2d has a higher resistence to these attacks because it is data dependent. The library used was the `golang.org/x/crypto/argon2` implementation. This library does NOT expose Argon2d because it is actually vulnerable to side channel attacks. Thankfully this project is not hashing passwords so we can use it. This repo was copied and modified with source in the crypto module of this project.

The settled on configuration for argon is: 512MB and 2 threads.

### Wallet pub/piv keys

The asymetric keys type used are Ed25519 Elliptic curve keys. The wallet address is equivalent to the public key. The public key gets transmitted as the hex output of the X, Y values concatted together. Because of the EC algorithm the length is fine for wallet addresses and the lengths are consistently 32 bits.

The signiature for signed data is transmitted as string value of the two value signiatures. This is R.S where the two components of hash are separated between a period. These values are parsed and used to validate the authenticity of the transaction data.

## Transactions

The transaction follows the following format, this gives the node enough information to verify the transaction.

```golang
type Transaction struct {
  Sig string `json:"sig"`
  
  Coinbase bool   `json:"coinbase"`
  Note     string `json:"note"`
  Nonce    int64  `json:"nonce"`
  To       string `json:"to"`
  From     string `json:"from"`
  Amount   int64  `json:"amount"`
}
```

The order of the data is very important for consistent signature. We use the order as below with no spaces:

`coinbase` `note` `nonce` `to` `from` `amount`

This data is signed by the private key and added to the signature value.

Only one coinbase txn is allowed per block. This should be written by the miner.

The FIRST txn nonce from a user must be 1.

## Blocks

To serialize we use the order as below with no spaces:

`timestamp` `height` `nonce` `previousHash` `transactions`

The transactions are serialized after the rest of the data, the order is the string alphabetical order.

## Broombase

The Broombase is a database containing a series of files with raw bytes that marshall/unmarshall into json. Each file represents a block and contains all of the transactions for the block. The exposed fucntions are wrapped in a RWmutex to structure concurrent operations. Forked transactions can also be stored in the broombase. The highest block function resolves forks automatically. A strategy to resolve which is highest is implemented in code.

Each file in the Broombase is labelled in the following format:

`{height}_{hash}.broom`

## Ledger

The ledger is an in memory balance summary for each account. This is settled by stacking each block from source, validating and calculating final balances and highest nonces. Snapshots of the ledger are saved. These are stored similarly to broombase. The ledger is stored in the file format:

`{height}_{hash}.broomledger`

Ledger snapshots are needed to resolve forks. The node needs the ability to validate blocks on a whim. Usually block validation is for current blocks. In the case of a fork, block validation will have to happen on all levels, this way arbitrary blocks can be added. The only condition to this is, we MUST have the previous block. Logic will be added to reconcile missing blocks in the event of a large fork.

## Fork Tracking

The flow is as follows, we receive a block. If this block solves the current puzzle then we add it, updating the ledger. If it does not, we need to locate the previous ledger snapshot. We validate the block against this snapshot, accumulate the ledger on the validated block, and then save the accumulated ledger to the file to continue the fork.

## Executor

The executor runs a loop of mining, then breaks the loop when a new txn or block comes in. This block is added or txn is added to the chain or mining block and mempool respectively. Then mining continues. When a block is found and proven to be valid, we want to make sure the block solves our current solution and clear those txn out of the mempool. When restarting on top of that block we pull in all the remaining mempool txns.

## Network Syncing

The network sync logic is somewhat inefficient because of the argon2 hashrate. The advised method to sync will be to get a snapshot of ledger and block data from a trusted source. I will be posting snapshot tar files of the entire network.

### Sync Mechanics

We run a network sync that will sample the network for the highest block. We trace backwards from this block until we collide witha block we already have. This process is repeated until we agree with the network on highest block. This is because while syncing; the network may have progressed forward.

### Other Sync Points

There is an edge case that exists where the network gets ahead of the current node by over one block. If this is the case we have a missing block. We will have to run a network sync to catch back up. This jumped block must be confirmed by multiple sources to proceed. Otherwise we do not want to waste compute on syncing forward to bad blocks. The plan is; we periodically sync to the network based on a time interval (5 min), this will stop malicious attackers from stopping mining progress.

## Egress

The network needs to distribute valid txns and blocks. The default node behavior will be to broadcast valid txns and blocks. This doe snot include txns that have already been added. We verify, add to our own ledger, blockchain, mempool; then we broadcast to our peers the good data.

## Mempool

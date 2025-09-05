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

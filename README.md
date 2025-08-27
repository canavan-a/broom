# Broom

A distributed ledger written in Go based loosely on the Satoshi Nakamotot bitcoin whitepaper. Credit also given to Gary Gensler's course on MIT openCourseWare: Blockchain and Money.

## Background

Broom is not a token. It is the ledger and blockchain infrastructure that will support a future, unamed token. The name comes from history: in Medieval England, tally sticks were used to record debts by carving marks into wood. A broom, being a bundle of sticks, represents strength and resilience through unity. In Broom, each node is like a stick, individually small but collectively forming a secure, reliable system. Even if one fails, the whole network remains effective.

## Overview

The ledger is backed by Ed25519 Elliptic curve key signiatures for all cryptography. Addresses are generated from the public key and Kekccak256 hash. Broom uses JSON messages over TCP to transmit information between nodes. The starting verification algorithm is Proof of Work using a nunce formula. Proof of Stake will be implemented at a later date.

## Node messaging protocol

Nodes communicate on port 4188 and messages are passed via a bytestream of encoded json. Each message starts with a unique byte delimeter 276

protocol looks like:

`276` `8 bytes length value` `data (however long the length value says)`

## Cryptography

The asymetric keys type used are Ed25519 Elliptic curve keys. The wallet address is equivalent to the public key. The public key gets transmitted as the hex output of the X, Y values concatted together. because if the EC algorithm the length is fine for wallet addresses.

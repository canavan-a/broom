package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"math/big"

	nodecrypto "github.com/canavan-a/broom/node/crypto"
)

type Wallet struct {
	NodePool []string

	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
}

func MakeWallet(nodes ...string) *Wallet {
	return &Wallet{
		NodePool: nodes,
	}
}

func (w *Wallet) Init(address, privateKey string) {

	pk, err := nodecrypto.ParsePublicKey(address)
	if err != nil {
		fmt.Println("could not parse pub key")
		panic(err)
	}

	w.PublicKey = pk

	privD := new(big.Int)
	privD.SetString(privateKey, 16)

	w.PrivateKey = &ecdsa.PrivateKey{
		PublicKey: *w.PublicKey,
		D:         privD,
	}
}

// creates new wallet and Initializes the client to that value
func (w *Wallet) NewKeypair() (address string, privateKey string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	address, err = nodecrypto.GenerateAddress(priv.PublicKey)
	if err != nil {
		return "", "", err
	}

	privateKey = nodecrypto.GeneratePrivateKeyText(priv)

	w.Init(address, privateKey)

	return
}

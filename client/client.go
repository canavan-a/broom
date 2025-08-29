package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"math/big"

	nodecrypto "github.com/canavan-a/broom/node/crypto"
)

type Client struct {
	NodePool []string

	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
}

func NewClient(nodes ...string) *Client {
	return &Client{
		NodePool: nodes,
	}
}

func (c *Client) Init(address, privateKey string) {

	pk, err := nodecrypto.ParsePublicKey(address)
	if err != nil {
		fmt.Println("could not parse pub key")
		panic(err)
	}

	c.PublicKey = pk

	privD := new(big.Int)
	privD.SetString(privateKey, 16)

	c.PrivateKey = &ecdsa.PrivateKey{
		PublicKey: *c.PublicKey,
		D:         privD,
	}
}

// creates new wallet and Initializes the client to that value
func (c *Client) NewWallet() (address string, privateKey string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	address, err = nodecrypto.GenerateAddress(priv.PublicKey)
	if err != nil {
		return "", "", err
	}

	privateKey = nodecrypto.GeneratePrivateKeyText(priv)

	c.Init(address, privateKey)

	return
}

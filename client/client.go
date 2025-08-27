package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

	c.PublicKey = nodecrypto.ParsePublicKey(address)

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

	address = nodecrypto.GenerateAddress(priv.PublicKey)

	privateKey = priv.D.Text(16)

	c.Init(address, privateKey)

	return
}

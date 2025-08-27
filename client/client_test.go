package main

import (
	"fmt"
	"testing"

	nodecrypto "github.com/canavan-a/broom/node/crypto"
)

func TestNewWallet(t *testing.T) {

	client := NewClient()

	privateKey, address, err := client.NewWallet()
	if err != nil {
		t.Error(err)
	}

	fmt.Println("address: ", address)

	fmt.Println("privateKey: ", privateKey)

}

func TestSignatures(t *testing.T) {
	client := NewClient()

	_, _, err := client.NewWallet()
	if err != nil {
		t.Error(err)
	}

	data := []byte("hello world!!!")

	signature := nodecrypto.Sign(data, client.PrivateKey)

	if !nodecrypto.Verify(data, signature, *client.PublicKey) {
		t.Error("could not validate signature")
	}

	tamperedData := []byte("evil hello world")
	if nodecrypto.Verify(tamperedData, signature, *client.PublicKey) {
		t.Error("varified invalid signature")
	}

}

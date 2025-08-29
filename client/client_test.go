package main

import (
	"fmt"
	"testing"

	nodecrypto "github.com/canavan-a/broom/node/crypto"
)

func TestNewWallet(t *testing.T) {

	client := NewClient()

	address, privateKey, err := client.NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("addresss: %s\n", address)

	fmt.Printf("pkey: %s", privateKey)

}

func TestSignatures(t *testing.T) {
	// client := NewClient()

	// address, pk, err := client.NewWallet()
	// if err != nil {
	// 	t.Error(err)
	// }

	address := "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEo9QDtE5lWkdpLXlIsLKtDsrmXuoGTwNexouisLP558XzqsDNNiT3VLFtXIxWrTHu7hni9LfnAQyRo3+7CyzXrQ=="
	// pub, err := nodecrypto.ParsePublicKey(address)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	pk := "MHcCAQEEIFMgAbTXECVfKIjf+kM2dpYNmuE8GRyOeKCBGjYXNOTooAoGCCqGSM49AwEHoUQDQgAEo9QDtE5lWkdpLXlIsLKtDsrmXuoGTwNexouisLP558XzqsDNNiT3VLFtXIxWrTHu7hni9LfnAQyRo3+7CyzXrQ=="

	pub, err := nodecrypto.ParsePublicKey(address)
	if err != nil {

		t.Fatal(err.Error())
	}

	PrivateKey, err := nodecrypto.ParsePrivateKey(pk)
	if err != nil {
		t.Fatal(err.Error())
	}

	if PrivateKey == nil {
		t.Fatal(err.Error())
	}

	data := []byte("hello worsld!!!")

	signature := nodecrypto.Sign(data, PrivateKey)

	if !nodecrypto.Verify(data, signature, pub) {
		t.Error("could not validate signature")
	}

	tamperedData := []byte("evil hello world")
	if nodecrypto.Verify(tamperedData, signature, pub) {
		t.Error("varified invalid signature")
	}

}

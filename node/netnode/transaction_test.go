package netnode

import (
	"fmt"
	"testing"

	"github.com/canavan-a/broom/node/crypto"
)

func TestTransaction(t *testing.T) {

	privateKey := "MHcCAQEEIFMgAbTXECVfKIjf+kM2dpYNmuE8GRyOeKCBGjYXNOTooAoGCCqGSM49AwEHoUQDQgAEo9QDtE5lWkdpLXlIsLKtDsrmXuoGTwNexouisLP558XzqsDNNiT3VLFtXIxWrTHu7hni9LfnAQyRo3+7CyzXrQ=="

	address := "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEo9QDtE5lWkdpLXlIsLKtDsrmXuoGTwNexouisLP558XzqsDNNiT3VLFtXIxWrTHu7hni9LfnAQyRo3+7CyzXrQ=="

	txn := Transaction{
		To:     "c41e918cebe58d0b127df69547a7cbf7f3bd09231e120f96692ca45f4d6bb0cd",
		From:   address,
		Nonce:  0,
		Amount: 500,
	}

	txn.Sign(privateKey)

	valid, err := txn.ValidateSig()
	if err != nil {
		t.Fatal(err)
	}

	if !valid {
		t.Fatal(err)
	}
}

func Sign(privateKey string, data []byte) string {

	Priv, err := crypto.ParsePrivateKey(privateKey)
	if err != nil {
		panic(err)
	}

	sig := crypto.Sign(data, Priv)
	fmt.Println("signed")

	return sig

}

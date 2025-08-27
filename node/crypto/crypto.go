package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"
)

func GenerateAddress(publicKey ecdsa.PublicKey) string {
	address := publicKey.X.Text(16) + publicKey.Y.Text(16)
	return address
}

func ParsePublicKey(address string) *ecdsa.PublicKey {

	mid := len(address) / 2
	X := address[:mid]
	Y := address[mid:]

	pubX := new(big.Int)
	pubX.SetString(X, 16)

	pubY := new(big.Int)
	pubY.SetString(Y, 16)

	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     pubX,
		Y:     pubY,
	}
}

func Sign(data []byte, privateKey *ecdsa.PrivateKey) (signi string) {

	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		panic(err)
	}

	signi = fmt.Sprintf("%x.%x", r, s)

	return
}

func Verify(data []byte, signi string, publicKey ecdsa.PublicKey) bool {
	parts := strings.Split(signi, ".")
	if len(parts) != 2 {
		return false
	}

	r := new(big.Int)
	s := new(big.Int)
	r.SetString(parts[0], 16)
	s.SetString(parts[1], 16)

	hash := sha256.Sum256(data)
	return ecdsa.Verify(&publicKey, hash[:], r, s)
}

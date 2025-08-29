package crypto

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
)

// Address = public key as base64
func GenerateAddress(publicKey ecdsa.PublicKey) (string, error) {
	b, err := x509.MarshalPKIXPublicKey(&publicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func ParsePublicKey(address string) (*ecdsa.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(address)
	if err != nil {
		return nil, err
	}

	key, err := x509.ParsePKIXPublicKey(b)
	if err != nil {
		return nil, err
	}

	pub, ok := key.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an ECDSA public key")
	}
	return pub, nil
}

func GeneratePrivateKeyText(privateKey *ecdsa.PrivateKey) string {
	b, _ := x509.MarshalECPrivateKey(privateKey)
	return base64.StdEncoding.EncodeToString(b)
}

func ParsePrivateKey(privateKeyText string) (*ecdsa.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(privateKeyText)
	if err != nil {
		return nil, err
	}
	return x509.ParseECPrivateKey(b)
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

func Verify(data []byte, signi string, publicKey *ecdsa.PublicKey) bool {
	parts := strings.Split(signi, ".")
	if len(parts) != 2 {
		return false
	}

	r := new(big.Int)
	s := new(big.Int)
	if _, ok := r.SetString(parts[0], 16); !ok {
		return false
	}
	if _, ok := s.SetString(parts[1], 16); !ok {
		return false
	}

	hash := sha256.Sum256(data)
	return ecdsa.Verify(publicKey, hash[:], r, s)
}

package netnode

import (
	"bytes"
	"encoding/binary"

	"github.com/canavan-a/broom/node/crypto"
)

type Transaction struct {
	Sig string `json:"sig"`

	Nonce  int64  `json:"nonce"`
	To     string `json:"to"`
	From   string `json:"from"`
	Amount int64  `json:"amount"`
}

func (t *Transaction) Serialize() []byte {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, t.Nonce)
	buf.WriteString(t.To)
	buf.WriteString(t.From)
	binary.Write(buf, binary.BigEndian, t.Amount)

	return buf.Bytes()
}

func (t *Transaction) Sign(privateKey string) error {
	serialized := t.Serialize()

	pkey, err := crypto.ParsePrivateKey(privateKey)
	if err != nil {
		return err
	}
	signed := crypto.Sign(serialized, pkey)

	t.Sig = signed

	return nil
}

func (t *Transaction) ValidateSig() (bool, error) {

	pub, err := crypto.ParsePublicKey(t.From)
	if err != nil {
		return false, err
	}

	return crypto.Verify(t.Serialize(), t.Sig, pub), nil
}

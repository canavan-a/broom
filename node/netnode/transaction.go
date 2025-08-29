package netnode

import (
	"bytes"
	"encoding/binary"
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

func (t *Transaction) Sign(privateKey string) {
	// serialized := t.Serialize()

}

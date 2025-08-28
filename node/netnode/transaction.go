package netnode

type Transaction struct {
	Sig string `json:"sig"`

	Nonce int64 `json:"nonce"`

	To   string `json:"to"`
	From string `json:"from"`

	Amount string `json:"amount"`
}

func (t *Transaction) Serialize() []byte {

	return []byte{}
}

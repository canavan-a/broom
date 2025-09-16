package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"sync"

	nodecrypto "github.com/canavan-a/broom/node/crypto"
)

const WALLET_FILENAME = "walletconfig.broom"

const WALLET_DATA_FILENAME = "walletdata.broom"

type Wallet struct {
	Seeds      []string
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey

	Balance int
	Nonce   int
}

type Config struct {
	PrivateKey string   `json:"private"`
	PublicKey  string   `json:"public"`
	Seeds      []string `json:"seeds"`
}

type Data struct {
	Balance int `json:"balance"`
	Nonce   int `json:"nonce"`
}

func MakeWallet() *Wallet {
	w := &Wallet{}

	err := w.LoadConfig()
	if err != nil {
		fmt.Println("could not load keypair from cfg")
	}

	err = w.LoadData()
	if err != nil {
		fmt.Println("could not load keypair from cfg")
	}

	return w
}

func (w *Wallet) Run() {
	flag.Parse()
	args := flag.Args()

	switch args[0] {
	case "sync":
		w.SyncBalance()
		fmt.Println("balance synced")
		fmt.Println("Balance: ", float64(w.Balance)/1000.0)
	case "balance":
		fmt.Println("Balance: ", float64(w.Balance)/1000.0)
	case "address":
		key, err := nodecrypto.GenerateAddress(*w.PublicKey)
		if err != nil {
			panic(err)
		}
		fmt.Println(key)
	case "key-rotate":
		_, _, err := w.NewKeypair()
		if err != nil {
			panic(err)
		}
		w.WriteData(Data{})
		fmt.Println("keys rotated")
	case "add-seeds":
		w.Seeds = append(w.Seeds, args[1:]...)
	case "clear-seeds":
		w.Seeds = []string{}
	default:
		fmt.Println("invalid command")
	}
}

func (w *Wallet) SyncBalance() error {

	fmt.Println("syncing")
	type NonceBalanceVote struct {
		nonce   int64
		balance int64
		Vote    int64
	}

	results := make(map[int64]NonceBalanceVote)
	mut := sync.Mutex{}

	address, err := nodecrypto.GenerateAddress(*w.PublicKey)
	if err != nil {
		return err
	}

	for _, seed := range w.Seeds {
		go func() {
			balance, nonce, err := w.CheckSeedBalance(seed, address)
			if err == nil {
				mut.Lock()
				bal := results[balance]
				results[balance] = NonceBalanceVote{
					nonce:   nonce,
					balance: balance,
					Vote:    bal.Vote + 1,
				}
				mut.Unlock()
			}
		}()
	}

	var winner NonceBalanceVote

	for _, candidate := range results {
		if candidate.Vote > winner.Vote {
			winner = candidate
		}
	}

	w.Balance = int(winner.balance)

	w.Nonce = int(winner.nonce)

	w.WriteData(Data{
		Nonce:   w.Nonce,
		Balance: w.Balance,
	})

	return nil

}

func (w *Wallet) CheckSeedBalance(seed, address string) (int64, int64, error) {
	type Payload struct {
		Address string `json:"address"`
	}

	payload := Payload{
		Address: address,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return 0, 0, err
	}

	type Response struct {
		Balance int64 `json:"balance"`
		Nonce   int64 `json:"nonce"`
	}

	resp, err := http.Post(fmt.Sprintf("http://%s/address", seed), "application/json", bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	var res Response

	err = json.Unmarshal(respBytes, &res)
	if err != nil {
		return 0, 0, err
	}

	return res.Balance, res.Nonce, nil
}

func (w *Wallet) Init(address, privateKey string) {

	pk, err := nodecrypto.ParsePublicKey(address)
	if err != nil {
		fmt.Println("could not parse pub key")
		panic(err)
	}

	w.PublicKey = pk

	privD := new(big.Int)
	privD.SetString(privateKey, 16)

	w.PrivateKey = &ecdsa.PrivateKey{
		PublicKey: *w.PublicKey,
		D:         privD,
	}
}

// creates new wallet and Initializes the client to that value
func (w *Wallet) NewKeypair() (address string, privateKey string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	address, err = nodecrypto.GenerateAddress(priv.PublicKey)
	if err != nil {
		return "", "", err
	}

	privateKey = nodecrypto.GeneratePrivateKeyText(priv)

	w.Init(address, privateKey)

	w.WriteConfig(Config{
		PublicKey:  address,
		PrivateKey: privateKey,
	})

	return
}

func (w *Wallet) LoadConfig() error {
	file, err := os.ReadFile(WALLET_FILENAME)
	if err != nil {
		w.WriteConfig(Config{
			PublicKey:  "",
			PrivateKey: "",
			Seeds:      []string{},
		})
		fmt.Println("no config file found, generating...")
		w.LoadConfig()
		return err
	}
	var cfg Config
	err = json.Unmarshal(file, &cfg)
	if err != nil {
		panic(err)
	}

	w.Seeds = cfg.Seeds

	priv, err := nodecrypto.ParsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return err
	}

	pub, err := nodecrypto.ParsePublicKey(cfg.PublicKey)
	if err != nil {
		return err
	}

	w.PrivateKey = priv
	w.PublicKey = pub

	return nil

}

func (w *Wallet) LoadData() error {
	file, err := os.ReadFile(WALLET_DATA_FILENAME)
	if err != nil {
		w.WriteData(Data{})
		fmt.Println("no data file found, generating...")
		w.LoadConfig()
		return err
	}
	var dat Data
	err = json.Unmarshal(file, &dat)
	if err != nil {
		panic(err)
	}

	w.Balance = dat.Balance
	w.Nonce = dat.Nonce

	return nil

}

func (w *Wallet) WriteConfig(cfg Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	_, fileErr := os.Open(WALLET_FILENAME)
	if fileErr == nil {

		if os.Rename(WALLET_FILENAME, fmt.Sprintf("old_%s", WALLET_FILENAME)) != nil {
			panic("cannot make old filename")
		}
	}

	err = os.WriteFile(WALLET_FILENAME, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (w *Wallet) WriteData(dat Data) error {
	data, err := json.Marshal(dat)
	if err != nil {
		return err
	}

	err = os.WriteFile(WALLET_DATA_FILENAME, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

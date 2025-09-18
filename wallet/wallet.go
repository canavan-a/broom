package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	nodecrypto "github.com/canavan-a/broom/node/crypto"
	netnode "github.com/canavan-a/broom/node/netnode"
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
	case "init":
		if !w.AreYouSure("init, this will overwrite existing keys") {
			return
		}
		w.CliInit()
	case "sync":
		w.SyncBalance()
		fmt.Println("balance synced")
		fmt.Println("Balance: ", float64(w.Balance)/1000.0)
	case "balance":
		fmt.Println("Run sync to update network")
		fmt.Println("Balance: ", float64(w.Balance)/1000.0)
	case "address":
		key, err := nodecrypto.GenerateAddress(*w.PublicKey)
		if err != nil {
			panic(err)
		}
		fmt.Println(key)
	case "send":
		w.Send()
		fmt.Println("Send Broom")
	case "generate-keys":
		if !w.AreYouSure("generating new keys, this will overwrite existing keys") {
			return
		}
		_, _, err := w.NewKeypair()
		if err != nil {
			panic(err)
		}
		w.WriteData(Data{})
		fmt.Println("keys rotated")
	case "seeds":
		if len(args) == 1 {
			fmt.Println("options: add, list, clear")
			return
		}
		switch args[1] {
		case "add":
			if len(args) == 2 {
				fmt.Println("please supply command with addresses/hostnames")
			}

			for _, seed := range args[2:] {
				if !slices.Contains(w.Seeds, seed) {
					w.Seeds = append(w.Seeds, seed)
				}
			}
			w.SaveSeeds()
			fallthrough
		case "list":
			for _, seed := range w.Seeds {
				fmt.Println(seed)
			}
		case "clear":
			if !w.AreYouSure("clear seeds") {
				return
			}
			w.Seeds = []string{}
			w.SaveSeeds()
		default:
			fmt.Println("invalid command")
		}
	case "contact":
		fmt.Println("Website:")
		fmt.Println("https://broomledger.com/")
		fmt.Println("Email:")
		fmt.Println("info@broomledger.com")
	default:
		fmt.Println("invalid command")
	}
}

func (w *Wallet) SaveSeeds() {
	file, err := os.ReadFile(WALLET_FILENAME)
	if err != nil {
		panic("could not open file")
	}
	var cfg Config
	err = json.Unmarshal(file, &cfg)
	if err != nil {
		panic(err)
	}

	cfg.Seeds = w.Seeds

	data, err := json.Marshal(cfg)
	if err != nil {
		panic("how did we get here")
	}

	err = os.WriteFile(WALLET_FILENAME, data, 0644)
	if err != nil {
		panic("could not back up seeds")
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

	secureRequest := ""

	host := seed
	if net.ParseIP(host) == nil {
		secureRequest = "s"
	}

	resp, err := http.Post(fmt.Sprintf("http%s://%s/address", secureRequest, seed), "application/json", bytes.NewReader(data))
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

	if _, err := os.Stat(WALLET_FILENAME); err == nil {
		if err := os.Rename(WALLET_FILENAME, "old_"+WALLET_FILENAME); err != nil {
			panic(fmt.Sprintf("cannot rename wallet file: %v", err))
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

func (w *Wallet) AreYouSure(operation string) bool {
	fmt.Printf("Are you sure you want to %s?\n", operation)
	fmt.Println("Y/n")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		line := scanner.Text()
		if strings.Trim(strings.ToLower(line), " ") == "y" {
			return true
		}
	}
	if err := scanner.Err(); err != nil {
		log.Println("Error reading:", err)
	}

	return false
}

func (w *Wallet) CliInit() {

	var cfg Config

	fmt.Println("Enter wallet address or public key:")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		line := scanner.Text()
		cfg.PublicKey = strings.Trim(line, " ")
		_, err := nodecrypto.ParsePublicKey(cfg.PublicKey)
		if err != nil {
			fmt.Println("Invalid public key")
			return
		}
	}

	fmt.Println("Enter private key:")
	if scanner.Scan() {
		line := scanner.Text()
		cfg.PrivateKey = strings.Trim(line, " ")
		_, err := nodecrypto.ParsePrivateKey(cfg.PrivateKey)
		if err != nil {
			fmt.Println("Invalid private key")
			return
		}
	}

	w.WriteConfig(cfg)
	fmt.Println("New config saved.\n\n Run sync to get current balance.")
}

func (w *Wallet) Send() {

	var tmpAmt float64

	var txn netnode.Transaction

	pub, err := nodecrypto.GenerateAddress(*w.PublicKey)
	if err != nil {
		fmt.Println("onvalid public key")
		return
	}

	txn.From = pub

	txn.Nonce = int64(w.Nonce) + 1

	fmt.Println("Enter recipient address:")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		line := strings.Trim(scanner.Text(), " ")
		txn.To = line
	}

	for {

		fmt.Println("Enter amount: ")
		if scanner.Scan() {
			line := strings.Trim(scanner.Text(), " ")

			amt, err := strconv.ParseFloat(line, 64)
			if err != nil {
				fmt.Println("invalid amount")
				return
			}

			tmpAmt = amt

			txn.Amount = int64(amt * 1000)

			if txn.Amount > int64(w.Balance) {
				fmt.Println("Not enough funds.")
			} else if txn.Amount == 0 {
				fmt.Println("Cannot send 0")
			} else {
				break
			}
		}
	}

	fmt.Println("Enter note: ")
	if scanner.Scan() {
		line := strings.Trim(scanner.Text(), " ")
		txn.Note = line
	}

	fmt.Println("Preview:")
	fmt.Println("To: ", txn.To)
	fmt.Println("Amount: ", tmpAmt)
	fmt.Println("Note: ", txn.Note)

	if !w.AreYouSure(" send money") {
		return
	}

	priv := nodecrypto.GeneratePrivateKeyText(w.PrivateKey)

	txn.Sign(priv)

	if !txn.ValidateSize() {
		fmt.Println("Txn is too large to send")
	}

	w.BroadcastTxns(txn)

}

func (w *Wallet) BroadcastTxns(txn netnode.Transaction) {
	fmt.Println("txn sent")

	wg := sync.WaitGroup{}

	for _, address := range w.Seeds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := w.BroadcastTxn(address, txn)
			if err == nil {
				fmt.Printf("Txn broadcast to seed: %s", address)
			} else {
				fmt.Printf("Txn broadcast error on: %s", address)
			}

		}()

	}

	wg.Wait()

	fmt.Println("Txn broadcase complete")
}

func (w *Wallet) BroadcastTxn(address string, txn netnode.Transaction) error {
	secureRequest := ""

	host := address
	if net.ParseIP(host) == nil {
		secureRequest = "s"
	}

	// transaction

	data, err := json.Marshal(txn)
	if err != nil {
		return err
	}

	resp, err := http.Post(fmt.Sprintf("http%s://%s/address", secureRequest, address), "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		return errors.New("bad request")
	}

	return nil
}

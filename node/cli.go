package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/canavan-a/broom/node/netnode"
)

const CONFIG_FILENAME = "config.broom"

type Cli struct {
	config Config
	ex     *netnode.Executor
}

type Config struct {
	Address string   `json:"address"`
	Note    string   `json:"note"`
	Seeds   []string `json:"seeds"`
}

func NewCli() *Cli {

	cli := &Cli{}

	cli.LoadConfig()

	return cli
}

func (cli *Cli) initExecutor() {
	cli.ex = netnode.NewExecutor(cli.config.Address, cli.config.Note, "", "")
}

func (cli *Cli) LoadConfig() {
	file, err := os.ReadFile(CONFIG_FILENAME)
	if err != nil {
		cli.WriteConfig(Config{
			Seeds: []string{},
		})
		fmt.Println("no config file found, generating...")
		cli.LoadConfig()
		return
	}
	var cfg Config
	err = json.Unmarshal(file, &cfg)
	if err != nil {
		panic(err)
	}

	cli.config = cfg

}

func (cli *Cli) WriteConfig(cfg Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	err = os.WriteFile(CONFIG_FILENAME, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (cli *Cli) EditConfig(note, address string, seeds ...string) {

	if note != "" {
		cli.config.Note = note
	}

	if address != "" {
		cli.config.Address = address
	}

	cli.config.Seeds = append(cli.config.Seeds, seeds...)

	err := cli.WriteConfig(cli.config)
	if err != nil {
		panic(err)
	}

}

func (cli *Cli) ClearSeeds() {
	cli.config.Seeds = []string{}
	err := cli.WriteConfig(cli.config)
	if err != nil {
		panic(err)
	}

}

func (cli *Cli) Run() {

	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		fmt.Println("expected subcommand")
		return
	}

	switch args[0] {
	case "config":
		cli.DoConfig(args[1:])
	case "start":
		if !cli.ValidateFlags() {
			return
		}
		cli.initExecutor()
		if len(args) >= 3 {
			if args[1] == "workers" {
				wrkrs, err := strconv.Atoi(args[2])
				if err == nil && wrkrs != 0 {
					fmt.Printf("Starting with %d workers.\n", wrkrs)
					cli.ex.Start(wrkrs, cli.config.Seeds...)
				}
			} else {
				fmt.Println("invalid workers flag")
			}
		} else {
			fmt.Println("Starting node")
			cli.ex.Start(netnode.EXECUTOR_WORKER_COUNT, cli.config.Seeds...)
		}
	case "backup":
		cli.initExecutor()
		fmt.Println("backing up")
		cli.ex.RunBackup()
	default:
		fmt.Println("invalid command")
	}

}

func (cli *Cli) DoConfig(s []string) {
	switch s[0] {
	case "address":
		cli.EditConfig("", s[1])
	case "note":
		cli.EditConfig(s[1], "")
	case "seeds":
		if s[1] == "clear" {
			cli.ClearSeeds()
		} else {
			cli.EditConfig("", "", s[1:]...)
		}
	case "show":
		fmt.Println("Address: ", cli.config.Address)
		fmt.Println("Note: ", cli.config.Note)
		fmt.Println("Seeds: ", cli.config.Seeds)
	default:
		fmt.Println("options: address, note, show")
	}
}

func (cli *Cli) ValidateFlags() bool {
	if cli.config.Address == "" {
		fmt.Println("address is empty")
		fmt.Println("please run: broom config address")
		return false
	}

	if len(cli.config.Seeds) == 0 {
		fmt.Println("no seeds supplied")
		fmt.Println("please run: broom config seeds seed1 seed2 seed3 ....")
		return false
	}

	return true
}

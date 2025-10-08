package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

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
	ID      string   `json:"id"` //  this is your public url
	Port    string   `json:"port"`
}

func NewCli() *Cli {

	cli := &Cli{}

	cli.LoadConfig()

	return cli
}

func (cli *Cli) initExecutor() {
	cli.ex = netnode.NewExecutor(cli.config.Address, cli.config.Note, "", "", Version)
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

func (cli *Cli) EditConfig(note, address, id string, port string, seeds ...string) {

	if id != "" {
		cli.config.ID = id
	}

	if note != "" {
		cli.config.Note = note
	}

	if address != "" {
		cli.config.Address = address
	}

	if port != "" {
		cli.config.Port = port
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

func (cli *Cli) AreYouSure(operation string) bool {
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

func (cli *Cli) Run() {

	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		fmt.Println("expected subcommand")
		fmt.Println("run: 'broom help' for valid sub commands")
		return
	}

	switch args[0] {
	case "config":
		cli.DoConfig(args[1:])
	case "version":
		fmt.Println(Version)
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
					p := "80"
					if cli.config.Port != "" {
						p = cli.config.Port
					}
					fmt.Printf("starting server on port %s\n", p)
					cli.ex.SetPort(p)
					cli.ex.Start(wrkrs, cli.config.ID, cli.config.Seeds...)
				}
			} else {
				fmt.Println("invalid workers flag")
			}
		} else {
			fmt.Println("Starting node")
			cli.ex.Start(netnode.EXECUTOR_WORKER_COUNT, cli.config.ID, cli.config.Seeds...)
		}
	case "backup":
		cli.initExecutor()
		fmt.Println("backing up")
		if len(args) <= 2 {
			fmt.Println("flags: run, peer, file")
		}
		switch args[1] {
		case "run":
			cli.ex.RunBackup()
		case "peer":
			if !cli.AreYouSure("backup from network peer") {
				return
			}
			fmt.Println("backing up off peer")
			fmt.Println("peer: ", args[2])
			cli.ex.DownloadBackupFileFromPeer(args[2])
			fmt.Println("unzipping backup file")
			cli.ex.BackupFromFile("backup.tar.gz")
		case "file":
			if !cli.AreYouSure("backup from file") {
				return
			}
			fmt.Println("backing up off file")
			fmt.Println("file: ", args[2])
			cli.ex.BackupFromFile(args[2])
		}
	case "help":
		cmdHelp()
	default:
		fmt.Printf("Error: %v\n", args[0])
		fmt.Println("Invalid command")
		fmt.Println("Run 'broom help' for commands.")
	}

}

func cmdHelp() {
	fmt.Println("Configuration Commands run 'broom config':\n",
		"\taddress\n",
		"\tnote\n",
		"\tseeds\n",
		"\tid\n",
		"\tport\n",
		"\tshow\n",
		"\n\n",
		"Starting workers run 'broom start':\n",
		"\tadd a number for how many workers you want.\n",
		"\te.g. 'workers 2'\n",
		"\n\n",
		"Backing Up run 'broom backup':\n",
		"\trun\n",
		"\tpeer\n",
		"\tfile\n",
		"\tCheck node version: \n",
		"\tversion")
}

func (cli *Cli) DoConfig(s []string) {
	switch s[0] {
	case "address":
		cli.EditConfig("", s[1], "", "")
	case "note":
		cli.EditConfig(s[1], "", "", "")
	case "seeds":
		if s[1] == "clear" {
			cli.ClearSeeds()
		} else {
			cli.EditConfig("", "", "", "", s[1:]...)
		}
	case "id":
		cli.EditConfig("", "", s[1], "")
	case "port":
		cli.EditConfig("", "", "", s[1])
	case "show":
		fmt.Println("Address: ", cli.config.Address)
		fmt.Println("Note: ", cli.config.Note)
		fmt.Println("Seeds: ", cli.config.Seeds)
		fmt.Println("Id: ", cli.config.ID)
		fmt.Println("Port: ", cli.config.Port)
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

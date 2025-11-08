package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

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

	PoolTax    int64  `json:"poolTax"`
	PrivateKey string `json:"privateKey"`
	PoolNote   string `json:"poolNote"`

	Pool string `json:"pool"`
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

func (cli *Cli) EditConfig(note, address, id, port, privateKey, poolNote, pool string, poolTax int64, addingTax bool, seeds ...string) {

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

	if privateKey != "" {
		cli.config.PrivateKey = privateKey
	}

	if poolNote != "" {
		cli.config.PoolNote = poolNote
	}

	if pool != "" {
		cli.config.Pool = pool
	}
	if addingTax {
		cli.config.PoolTax = poolTax
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

type Command struct {
	Description string
	Run         func(cli *Cli, args []string)
}

type CommandFlag struct {
	Description string
	Action      func(cli *Cli)
}

var (
	commands      = map[string]*Command{}
	flagsRegistry = map[string]*CommandFlag{}
)
var globalFlagValues = map[string]*bool{}

func InitFlagsRegistry() {
	flagsRegistry["help"] = &CommandFlag{
		Description: "Display help information",
		Action:      func(cli *Cli) { PrintHelp() },
	}
	flagsRegistry["version"] = &CommandFlag{
		Description: "Display version information",
		Action:      func(cli *Cli) { fmt.Println(Version) },
	}
}

func InitCommandsRegistry() {
	commands["help"] = &Command{
		Description: "Show help for commands",
		Run: func(cli *Cli, args []string) {
			PrintHelp()
		},
	}
	commands["version"] = &Command{
		Description: "Show version",
		Run: func(cli *Cli, args []string) {
			fmt.Println(Version)
		},
	}
	commands["config"] = &Command{
		Description: "Configure broom (address|note|seeds|id|port|private-key|pool-note|pool-tax|show)",
		Run: func(cli *Cli, args []string) {
			if len(args) == 0 {
				fmt.Println("options: address, note, seeds, id, port, show")
				return
			}
			cli.DoConfig(args)
		},
	}
	commands["update"] = &Command{
		Description: "Update broom to the latest release, supply version flag to go to a specific version.",
		Run: func(cli *Cli, args []string) {
			if !cli.AreYouSure("update") {
				return
			}

			_, version := ParseSubFlag("version", args, true)
			fullCommand := "curl -sSL https://broomledger.com/install.sh | sudo bash"

			if version != "" {
				fullCommand = fmt.Sprintf("%s -s -- %s", fullCommand, version)
			}

			cmd := exec.Command("bash", "-c", fullCommand)
			out, err := cmd.CombinedOutput()
			fmt.Println(string(out))
			if err != nil {
				panic(err)
			}
		},
	}
	commands["start"] = &Command{
		Description: "Start the node (optionally: start workers <N>, or specify pool flag to enable mining pools) ",
		Run: func(cli *Cli, args []string) {
			if !cli.ValidateFlags() {
				return
			}
			cli.initExecutor()

			workers := netnode.EXECUTOR_WORKER_COUNT
			found, workerString := ParseSubFlag("workers", args, true)
			if found {
				i, err := strconv.Atoi(workerString)
				if err != nil {
					fmt.Println("invalid workers value")
					return
				}
				workers = i
			}

			poolEnabled, _ := ParseSubFlag("pool", args, false)
			if poolEnabled {
				if cli.config.PrivateKey == "" {
					fmt.Println("no private key configured")
				}
				if cli.ex.PoolTaxPercent == 0 {
					fmt.Println("WARNING: No pool tax enabled, pool mining will be fair")
					time.Sleep(time.Second)
				}
				cli.ex.EnableMiningPool(cli.config.PoolTax, cli.config.PrivateKey, cli.config.PoolNote)
			}

			fmt.Println("Starting node")
			cli.ex.Start(workers, cli.config.ID, cli.config.Seeds...)

		},
	}
	commands["backup"] = &Command{
		Description: "Backup operations (run|peer <addr>|file <path>)",
		Run: func(cli *Cli, args []string) {
			cli.initExecutor()
			fmt.Println("backing up")
			if len(args) == 0 {
				fmt.Println("flags: run, peer, file")
				return
			}
			switch args[0] {
			case "run":
				cli.ex.RunBackup()
			case "peer":
				if len(args) < 2 {
					fmt.Println("usage: backup peer <peer-address>")
					return
				}
				if !cli.AreYouSure("backup from network peer") {
					return
				}
				fmt.Println("backing up off peer")
				fmt.Println("peer: ", args[1])
				cli.ex.DownloadBackupFileFromPeer(args[1])
				fmt.Println("unzipping backup file")
				cli.ex.BackupFromFile("backup.tar.gz")
			case "file":
				if len(args) < 2 {
					fmt.Println("usage: backup file <backup-path>")
					return
				}
				if !cli.AreYouSure("backup from file") {
					return
				}
				fmt.Println("backing up off file")
				fmt.Println("file: ", args[1])
				cli.ex.BackupFromFile(args[1])
			default:
				fmt.Println("flags: run, peer, file")
			}
		},
	}
	commands["mine"] = &Command{
		Description: "Mine without running a full node yourself. Config needed: address and pool",
		Run: func(cli *Cli, args []string) {

			if cli.config.Pool == "" {
				fmt.Println("Pool is not configured, please run: broom config pool <pool address or ip>")
				return
			}

			workers := 1
			found, workersString := ParseSubFlag("workers", args, true)
			if found {
				workers, _ = strconv.Atoi(workersString)
				if workers == 0 {
					fmt.Println("invalid worker cound")
					return
				}
			}

			if cli.config.Address == "" {
				fmt.Println("please configure your address; broom config address <address>")
				return
			}

			miner := NewMiner(cli.config.Address, cli.config.Pool, workers)
			fmt.Println("connected to pool operator: ", cli.config.Pool)

			miner.Start()

		},
	}
}

func ParseSubFlag(flagName string, args []string, parseSuccessor bool) (found bool, successorValue string) {
	idx := slices.Index(args, flagName)
	if idx == -1 {
		return
	}
	if !parseSuccessor {
		return true, ""
	}

	if idx == len(args)-1 {
		return
	}

	return true, args[idx+1]

}

func RegisterFlags(fs *flag.FlagSet, cli *Cli) {
	for name, cf := range flagsRegistry {
		b := fs.Bool(name, false, cf.Description)
		globalFlagValues[name] = b
	}
}
func (cli *Cli) DoConfig(s []string) {
	if len(s) == 0 {
		fmt.Println("options: address, note, seeds, id, port, show")
		return
	}
	switch s[0] {
	case "address":
		cli.EditConfig("", s[1], "", "", "", "", "", 0, false)
	case "note":
		cli.EditConfig(s[1], "", "", "", "", "", "", 0, false)
	case "seeds":
		if s[1] == "clear" {
			cli.ClearSeeds()
		} else {
			cli.EditConfig("", "", "", "", "", "", "", 0, false, s[1:]...)
		}
	case "id":
		cli.EditConfig("", "", s[1], "", "", "", "", 0, false)
	case "port":
		cli.EditConfig("", "", "", s[1], "", "", "", 0, false)
	case "private-key":
		cli.EditConfig("", "", "", "", s[1], "", "", 0, false)
	case "pool-note":
		cli.EditConfig("", "", "", "", "", s[1], "", 0, false)
	case "pool":
		cli.EditConfig("", "", "", "", "", "", s[1], 0, false)
	case "pool-tax":
		val, err := strconv.Atoi(s[1])
		if err != nil {
			fmt.Println("invalid number")
			return
		}
		if val > 100 {
			fmt.Println("tax rate is too high")
			return
		}
		if val < 0 {
			fmt.Println("tax rate is too low")
			return
		}
		cli.EditConfig("", "", "", "", "", "", "", int64(val), true)
	case "show":
		fmt.Println("Address: ", cli.config.Address)
		fmt.Println("Note: ", cli.config.Note)
		fmt.Println("Seeds: ", cli.config.Seeds)
		fmt.Println("Id: ", cli.config.ID)
		fmt.Println("Port: ", cli.config.Port)
		fmt.Println("--- Pool config ---")
		fmt.Println("pool-note: ", cli.config.PoolNote)
		pk := cli.config.PrivateKey
		if len(pk) > 30 {
			fmt.Println("private-key: ", pk[:20], ".....", pk[len(pk)-20:])
		}
		fmt.Println("private-key: ", pk)
		fmt.Println("pool-tax: ", cli.config.PoolTax, "%")
		fmt.Println("--- Miner config ---")
		fmt.Println("pool: ", cli.config.Pool)
	default:
		fmt.Println("options: address, note, seeds, id, port, private-key, pool-note, pool-tax, pool, show")
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

func PrintHelp() {
	fmt.Println("==== Broom CLI Help ====")

	fmt.Println("\nCommands:")
	cmdNames := make([]string, 0, len(commands))
	for n := range commands {
		cmdNames = append(cmdNames, n)
	}
	sort.Strings(cmdNames)
	for _, name := range cmdNames {
		fmt.Printf("  %s \t%s\n", name, commands[name].Description)
	}

	fmt.Println("\nGlobal Flags:")
	flagNames := make([]string, 0, len(flagsRegistry))
	for n := range flagsRegistry {
		flagNames = append(flagNames, n)
	}
	sort.Strings(flagNames)
	for _, name := range flagNames {
		fmt.Printf("  -%s \t%s\n", name, flagsRegistry[name].Description)
	}

	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  broom start")
	fmt.Println("  broom start workers 4")
	fmt.Println("  broom config show")
	fmt.Println("  broom config address <ADDR>")
	fmt.Println("  broom backup run")
	fmt.Println("  broom backup peer 10.0.0.12:8080")
	fmt.Println("  broom backup file ./backup.tar.gz")
}

func (cli *Cli) Run() {
	fs := flag.NewFlagSet("broom", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	InitFlagsRegistry()
	InitCommandsRegistry()
	RegisterFlags(fs, cli)

	_ = fs.Parse(os.Args[1:])

	if runGlobalFlagActions(cli) {
		return
	}

	args := fs.Args()
	if len(args) < 1 {
		fmt.Println("expected subcommand")
		fmt.Println("run: 'broom help' for valid subcommands")
		return
	}

	cmdName := args[0]
	cmd, ok := commands[cmdName]
	if !ok {
		fmt.Printf("Error: %v\n", cmdName)
		fmt.Println("Invalid command")
		fmt.Println("Run 'broom help' for commands.")
		return
	}

	cmd.Run(cli, args[1:])
}

func runGlobalFlagActions(cli *Cli) (handled bool) {
	for name, v := range globalFlagValues {
		if v != nil && *v {
			if def, ok := flagsRegistry[name]; ok && def.Action != nil {
				def.Action(cli)
				handled = true
			}
		}
	}
	return
}

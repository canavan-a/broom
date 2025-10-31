package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
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
		Description: "Configure broom (address|note|seeds|id|port|show)",
		Run: func(cli *Cli, args []string) {
			if len(args) == 0 {
				fmt.Println("options: address, note, seeds, id, port, show")
				return
			}
			cli.DoConfig(args)
		},
	}
	commands["update"] = &Command{
		Description: "Update broom to the latest release",
		Run: func(cli *Cli, args []string) {
			if !cli.AreYouSure("update") {
				return
			}
			cmd := exec.Command("bash", "-c", "curl -sSL https://broomledger.com/install.sh | sudo bash")
			out, err := cmd.CombinedOutput()
			fmt.Println(string(out))
			if err != nil {
				panic(err)
			}
		},
	}
	commands["start"] = &Command{
		Description: "Start the node (optionally: start workers <N>)",
		Run: func(cli *Cli, args []string) {
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
		fmt.Println("options: address, note, seeds, id, port, show")
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
		fmt.Printf("  %s\t%s\n", name, commands[name].Description)
	}

	fmt.Println("\nGlobal Flags:")
	flagNames := make([]string, 0, len(flagsRegistry))
	for n := range flagsRegistry {
		flagNames = append(flagNames, n)
	}
	sort.Strings(flagNames)
	for _, name := range flagNames {
		fmt.Printf("  -%s\t%s\n", name, flagsRegistry[name].Description)
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

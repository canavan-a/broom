package main

var Version = "built from source"

func main() {
	cli := NewCli()
	cli.Run()
}

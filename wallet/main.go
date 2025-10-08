package main

var Version = "built from source"

func main() {
	w := MakeWallet()

	w.Run()
}

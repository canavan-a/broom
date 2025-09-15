module github.com/canavan-a/broom/wallet

go 1.25.0

require github.com/canavan-a/broom/node v0.0.0

require (
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
)

replace github.com/canavan-a/broom/node => ../node

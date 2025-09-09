package crypto

import (
	"fmt"
	"testing"
	"time"
)

func TestArgon2b(t *testing.T) {

	data := []byte("Hello world 189!")

	before := time.Now()
	hash := HashArgon2d(data)
	fmt.Println(time.Since(before))

	fmt.Println(hash)

}

package netnode

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {

	li := NewLimiter(time.Second, testerAction, testValidator)

	li.Publish("critical")
	li.Publish("hello")
	li.Publish("critical")
	li.Publish("hello")

	select {}

}

func testValidator(str string) bool {
	if str == "critical" {
		return true
	} else {
		return false
	}
}

func testerAction(input string) {
	if input == "critical" {
		fmt.Println("critical string!!!")
	} else {
		fmt.Println("regular string")
	}
}

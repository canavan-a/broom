package netnode

import (
	"fmt"
	"log"
	"net"
	"testing"
	"time"
)

func TestTcpMsg(t *testing.T) {

	go startTcp()
	time.Sleep(1 * time.Second)

	conn, err := net.Dial("tcp", net.JoinHostPort("0.0.0.0", TCP_PORT))
	if err != nil {
		log.Fatal("could not dial tcp locally")
	}

	peer := Peer{
		address: "0.0.0.0",
		conn:    conn,
	}

	respChannel := make(chan []byte)

	go peer.ListenProtocol(respChannel)

	for {
		data := <-respChannel

		if string(data) == "hello world" {
			break
		}
	}

}

func TestSample(t *testing.T) {
	fmt.Println("hhello")
}

func startTcp() {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", TCP_PORT))
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	conn, err := ln.Accept()
	if err != nil {
		log.Fatal("could not accept tcp conn")

	}

	for {
		fmt.Println("sending")
		//broadcast msg every 2 seconds
		msg := "hello world"

		data := signMsg([]byte(msg))

		_, err := conn.Write(data)
		if err != nil {
			fmt.Println("could not write data")
		}

		time.Sleep(2 * time.Second)
	}

}

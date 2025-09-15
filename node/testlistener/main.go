package main

import (
	"fmt"
	"log"
	"net"
)

func main() {
	fmt.Println("hello world")

	ln, err := net.Listen("tcp", ":4188")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Println("read error:", err)
			return
		}
		fmt.Println("received:", string(buf[:n]))
	}
}

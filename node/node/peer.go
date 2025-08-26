package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

type Peer struct {
	address string
	conn    net.Conn
}

func (p *Peer) ListenProtocol(msgChannel chan []byte) {
	var buffer []byte

	tmp := make([]byte, 1024)
	for {
		n, err := p.conn.Read(tmp)
		if err != nil {
			fmt.Println("disconnected from peer", err)
			return
		}
		buffer = append(buffer, tmp[:n]...)

		msgStart := bytes.Index(buffer, START_DELIMETER)
		if msgStart != -1 {
			buffer = buffer[msgStart:]

			if len(buffer) >= len(START_DELIMETER)+8 {
				messageLength := buffer[len(START_DELIMETER) : len(START_DELIMETER)+8]
				realLength := binary.BigEndian.Uint64(messageLength)

				// check against max size
				if realLength > PROTOCOL_MAX_SIZE {
					// too big
					buffer = buffer[len(START_DELIMETER):]

					fmt.Println("msg is too big")
				} else {
					if len(buffer) >= len(START_DELIMETER)+8+int(realLength) {
						start := len(START_DELIMETER) + 8
						end := len(START_DELIMETER) + 8 + int(realLength)

						msg := make([]byte, realLength)

						copy(msg, buffer[start:end])
						msgChannel <- msg

						buffer = buffer[end:]

					}
				}

			}

		}

	}
}

package netnode

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const TCP_PORT = "4188"

const RETRY_TIME = 3 // in seconds

const PEER_CONNECTION_RETRIES = 5

const MAX_PEER_CONNECTIONS = 200

const PROTOCOL_MAX_SIZE = 30_000_000

var START_DELIMETER = []byte{0x01, 0x14}

type MessageType string

const (
	Ping          MessageType = "Ping"
	PeerBroadcast MessageType = "PeerBroadcast"
)

type Node struct {
	seeds []string
	peers map[string]*Peer

	mutex      sync.Mutex
	msgChannel chan []byte
}

func ActivateNode(seedNodes ...string) *Node {

	node := &Node{
		seeds:      seedNodes,
		peers:      make(map[string]*Peer),
		mutex:      sync.Mutex{},
		msgChannel: make(chan []byte, 10),
	}

	// seed the supplied node ips
	node.Seed()

	// listen to incoming connections
	node.StartListenIncomingConnections()

	node.StartListenIncomingMessages()

	node.Schedule(node.BroadcastPeers, time.Minute*5)

	return node
}

func (n *Node) BroadcastPeers() {
	// prepare
}

func (n *Node) Seed() {
	for _, seed := range n.seeds {
		go n.SeedDial(seed)
	}
}

func (n *Node) SeedDial(seed string) {
	for {
		peer, err := n.DialPeer(seed)
		if err != nil {
			continue
		}

		n.mutex.Lock()
		n.peers[seed] = peer
		n.mutex.Unlock()

		peer.ListenProtocol(n.msgChannel)

		n.mutex.Lock()
		delete(n.peers, seed)
		n.mutex.Unlock()

		fmt.Println("retrying seed node dial")
		time.Sleep(RETRY_TIME * time.Second)

	}
}

func (n *Node) DialPeer(address string) (*Peer, error) {
	conn, err := net.Dial("tcp", net.JoinHostPort(address, TCP_PORT))
	if err != nil {
		return nil, err
	}

	peer := &Peer{
		start:   time.Now(),
		address: address,
		conn:    conn,
	}

	return peer, nil

}

func (n *Node) StartListenIncomingConnections() {
	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%s", TCP_PORT)) // listen on port 12345
		if err != nil {
			log.Fatal(err)
		}
		defer ln.Close()

		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Println("could not accept tcp conn")
				continue
			}
			go n.handleConn(conn)
		}
	}()

}

func (n *Node) handleConn(conn net.Conn) {
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return
	}

	defer conn.Close()

	n.mutex.Lock()
	// max check
	peerLength := len(n.peers)

	// check duplicates
	_, found := n.peers[host]

	n.mutex.Unlock()

	if found {
		return
	}

	if peerLength >= MAX_PEER_CONNECTIONS {
		return
	}

	retries := 0
	for {
		n.mutex.Lock()
		n.peers[host] = &Peer{
			start:   time.Now(),
			address: host,
			conn:    conn,
		}
		n.mutex.Unlock()

		n.peers[host].ListenProtocol(n.msgChannel)

		n.mutex.Lock()
		delete(n.peers, host)
		n.mutex.Unlock()
		if retries == PEER_CONNECTION_RETRIES {
			break
		}
		time.Sleep(RETRY_TIME * time.Second)
		retries++
	}

}

func (n *Node) addPeerFromHost(host string) {
	peer, err := n.DialPeer(host)
	if err != nil {
		fmt.Println("could not dial peer")
		return
	}

	go n.handleConn(peer.conn)
}

type Message struct {
	Action MessageType `json:"action"`

	HostNames []string `json:"hostname,omitempty"`
}

func (n *Node) StartListenIncomingMessages() {
	go func() {
		for {
			msg := <-n.msgChannel
			n.processIncomingMessage(msg)
		}
	}()
}

func (n *Node) broadcastMessageToPeers(rawMsg []byte) {
	signedMsg := signMsg(rawMsg)
	n.mutex.Lock()
	defer n.mutex.Unlock()
	for _, peer := range n.peers {
		_, err := peer.conn.Write(signedMsg)
		if err != nil {
			// do nothing
		}
	}
}

func (n *Node) Schedule(task func(), period time.Duration) {
	go func() {
		for {
			task()
			time.Sleep(period)
		}
	}()
}

func signMsg(msg []byte) []byte {
	base := make([]byte, 0, len(START_DELIMETER)+8+len(msg))

	base = append(base, START_DELIMETER...)

	lengthBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(lengthBytes, uint64(len(msg)))
	base = append(base, lengthBytes...)

	// Append the message
	base = append(base, msg...)

	return base
}

func (n *Node) processIncomingMessage(rawMsg []byte) {
	msg := Message{}
	err := json.Unmarshal(rawMsg, &msg)
	if err != nil {
		fmt.Println("invalid json format")
		return
	}

	switch msg.Action {
	case PeerBroadcast:
		for _, host := range msg.HostNames {
			n.addPeerFromHost(host)
		}
	case Ping:
		fmt.Println("pinged")
	}

}

func TestSomething() {

}

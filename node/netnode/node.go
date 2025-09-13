package netnode

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/canavan-a/broom/node/crypto"
)

const TCP_PORT = "4188"

const RETRY_TIME = 3 // in seconds

const PEER_CONNECTION_RETRIES = 5

const MAX_PEER_CONNECTIONS = 200

const PROTOCOL_MAX_SIZE = 30_000_000

const PEER_SAMPLE_SIZE = 30

const EXPOSED_PORT = "8080"

const NO_PEERS_ERROR = "no peers found"

var START_DELIMETER = []byte{0x01, 0x14}

type MessageType string

const (
	Ping            MessageType = "Ping"
	PeerBroadcast   MessageType = "PeerBroadcast"
	TransactionSend MessageType = "TransactionSend"
	BlockMsg        MessageType = "BlockMsg"
	BlockSolution   MessageType = "BlockSolution"
)

type Message struct {
	Action MessageType `json:"action"`

	HostNames []string `json:"hostname,omitempty"`
}

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

	node.Schedule(node.GossipPeers, time.Minute*5)

	return node
}

func (n *Node) GossipPeers() {
	// TODO: only broadcast a subset of peers out
	n.mutex.Lock()
	var peerHosts []string
	for _, peer := range n.peers {
		peerHosts = append(peerHosts, peer.address)
	}
	n.mutex.Unlock()

	hostsMessage := Message{
		Action:    PeerBroadcast,
		HostNames: peerHosts,
	}

	msg, err := json.Marshal(hostsMessage)
	if err != nil {
		log.Fatal("peer broadcast: invalid slice")
	}

	n.broadcastMessageToPeers(msg)
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
		n.peers[seed].conn.Close()
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

func (n *Node) StartListenIncomingMessages() {
	go func() {
		for {
			msg := <-n.msgChannel
			n.processIncomingMessage(msg)
		}
	}()
}

func (n *Node) broadcastMessageToPeers(rawMsg []byte) {
	formattedMsg := formatMsg(rawMsg)
	n.mutex.Lock()
	defer n.mutex.Unlock()
	for _, peer := range n.peers {
		_, err := peer.conn.Write(formattedMsg)
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

func formatMsg(msg []byte) []byte {
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
		n.BalancePeers(msg.HostNames)
	case Ping:
		fmt.Println("pinged")
	}

}

func (n *Node) BalancePeers(hostNames []string) {

	var currentPeers []string

	n.mutex.Lock()
	for peer := range n.peers {
		currentPeers = append(currentPeers, peer)
	}
	n.mutex.Unlock()

	var newPeers []string
	for _, host := range hostNames {
		if !slices.Contains(currentPeers, host) {
			newPeers = append(newPeers, host)
		}
	}

	for _, newPeer := range newPeers {
		if len(currentPeers)+1 > MAX_PEER_CONNECTIONS {

			randomNumber := rand.Intn(10) + 1 // 1–10
			if randomNumber == 10 {
				if n.DropRandomPeer() {
					n.addPeerFromHost(newPeer)
				}
			}

		} else {
			n.addPeerFromHost(newPeer)
		}
	}

}

func (n *Node) DropRandomPeer() bool {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	var randomPeer string
	for key := range n.peers {
		randomPeer = key
		break
	}
	if !slices.Contains(n.seeds, randomPeer) {
		n.peers[randomPeer].conn.Close()
		delete(n.peers, randomPeer)
		return true
	}

	return false
}

func (n *Node) requestPeerBlock(ctx context.Context, ipAddress string, path string, height int, hash string) (response Block, err error) {

	type Payload struct {
		Height int    `json:"height"`
		Hash   string `json:"hash"`
	}

	p := Payload{
		Height: height,
		Hash:   hash,
	}

	payloadData, err := json.Marshal(p)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s:%s/%s", ipAddress, EXPOSED_PORT, path), bytes.NewReader(payloadData))
	if err != nil {
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		err = errors.New("bad request")
		return

	}

	var block Block

	err = json.NewDecoder(resp.Body).Decode(&block)
	if err != nil {
		return
	}

	return
}

func (n *Node) SamplePeersBlock(path string, height int, hash string) (consensus Block, err error) {

	sample := n.GetAddressSample()

	wg := sync.WaitGroup{}
	mut := sync.Mutex{}

	ctx := context.Background()

	var sharedSample []Block

	for _, peerAddress := range sample {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			block, err := n.requestPeerBlock(ctx, addr, path, height, hash)
			if err == nil {
				mut.Lock()
				sharedSample = append(sharedSample, block)
				mut.Unlock()
			}

		}(peerAddress)
	}
	wg.Wait()

	if len(sharedSample) == 0 {
		err = errors.New("no block found")
		return
	}

	// check consensus and reconcile
	type BlockCount struct {
		Block Block
		Count int
	}

	summary := make(map[string]BlockCount)

	for _, value := range sharedSample {
		fastHash := crypto.Sha256Hash(value.Serialize())
		summary[fastHash] = BlockCount{
			Block: value,
			Count: summary[fastHash].Count + 1,
		}
	}

	var result Block
	var best int

	for _, value := range summary {
		if value.Count > best {
			result = value.Block
			best = value.Count
		}
	}

	return result, nil
}

func (n *Node) requestPeerHighestBlock(ctx context.Context, ipAddress string) (response HashHeight, err error) {

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s:%s/highest_block", ipAddress, EXPOSED_PORT), nil)
	if err != nil {
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		err = errors.New("bad request")
		return

	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return
	}

	return
}

func (n *Node) SamplePeersHighestBlock() (hash string, height int, err error) {

	sample := n.GetAddressSample()
	if len(sample) == 0 {
		return "", 0, errors.New(NO_PEERS_ERROR)
	}

	wg := sync.WaitGroup{}
	mut := sync.Mutex{}

	ctx := context.Background()
	var sharedSample []HashHeight

	for _, peerAddress := range sample {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			hh, err := n.requestPeerHighestBlock(ctx, addr)
			if err == nil {
				mut.Lock()
				sharedSample = append(sharedSample, hh)
				mut.Unlock()
			}

		}(peerAddress)
	}
	wg.Wait()

	if len(sharedSample) == 0 {
		err = errors.New("no hh found")
		return
	}

	// check consensus and reconcile
	type HashHeightCount struct {
		HashHeight HashHeight
		Count      int
	}

	summary := make(map[string]HashHeightCount)

	for _, value := range sharedSample {
		summary[value.Hash] = HashHeightCount{
			HashHeight: value,
			Count:      summary[value.Hash].Count + 1,
		}
	}

	var result HashHeight
	var best int

	for _, value := range summary {
		if value.Count > best {
			result = value.HashHeight
			best = value.Count
		}
	}

	return result.Hash, result.Height, nil
}

func (n *Node) GetAddressSample() []string {
	n.mutex.Lock()
	// // sample for n peer's addresses
	var allAddresses []string
	for _, peer := range n.peers {
		allAddresses = append(allAddresses, peer.address)
	}
	n.mutex.Unlock()

	rand.Shuffle(len(allAddresses), func(i, j int) {
		allAddresses[i], allAddresses[j] = allAddresses[j], allAddresses[i]
	})

	var sample []string
	if len(allAddresses) >= PEER_SAMPLE_SIZE {
		sample = allAddresses[0:PEER_SAMPLE_SIZE]
	} else {
		sample = allAddresses
	}

	return sample
}

func (n *Node) RacePeersForValidBlock(hash string, height int) (Block, error) {

	ctx := context.Background()

	var block Block
	var found bool

	sample := n.GetAddressSample()
	for _, address := range sample {
		// we cant "race" because our CPU would go nuts hashing argons
		foundBlock, err := n.requestPeerBlock(ctx, address, "block", height, hash)
		if err == nil {
			serialized := block.Serialize()
			if crypto.Hash(serialized) == hash {
				block = foundBlock
				found = true
				break
			}
		}

	}

	if found {
		return block, nil
	} else {
		return Block{}, errors.New("no block found, need to resample")
	}
}

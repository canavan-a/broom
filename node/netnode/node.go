package netnode

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const EXPOSED_PORT = "80"

const NO_PEERS_ERROR = "no peers found"

var START_DELIMETER = []byte{0x01, 0x14}

type MessageType string

const (
	Ping          MessageType = "Ping"
	PeerBroadcast MessageType = "PeerBroadcast"
	TxnMsg        MessageType = "TxnMsg"
	BlockMsg      MessageType = "BlockMsg"
)

type Message struct {
	Action MessageType `json:"action"`

	HostNames []string `json:"hostnames"`

	Block Block `json:"block"`

	Txn Transaction `json:"txn"`
}

// TODO: do egress logic

type Node struct {
	seeds []string
	peers map[string]*Peer

	mutex      sync.Mutex
	msgChannel chan []byte

	ingressBlock chan Block
	ingressTxn   chan Transaction

	egressBlock chan Block
	egressTxn   chan Transaction

	requestPeers map[string]*RequestPeer
}

type RequestPeer struct {
	ip      string
	strikes int
}

func ActivateNode(msgChannel chan []byte, ingressBlock, egressBlock chan Block, ingressTxn, egressTxn chan Transaction, seedNodes ...string) *Node {

	node := &Node{
		seeds:        seedNodes,
		peers:        make(map[string]*Peer),
		mutex:        sync.Mutex{},
		msgChannel:   msgChannel,
		ingressBlock: ingressBlock,
		ingressTxn:   ingressTxn,
		egressBlock:  egressBlock,
		egressTxn:    egressTxn,
	}

	// seed the supplied node ips
	node.Seed()

	node.RunEgress()

	node.Schedule(node.GossipPeers, time.Minute*5)

	return node
}

func (n *Node) GossipPeers() {

	fmt.Println("gossiping peers")
	// TODO: only broadcast a subset of peers out
	n.mutex.Lock()
	var peerHosts []string
	for _, peer := range n.requestPeers {
		peerHosts = append(peerHosts, peer.ip)
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
	n.mutex.Lock()
	defer n.mutex.Unlock()
	for _, seed := range n.seeds {
		n.requestPeers[seed] = &RequestPeer{
			ip: seed,
		}
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

	n.mutex.Lock()
	defer n.mutex.Lock()
	n.requestPeers[host] = &RequestPeer{
		ip: host,
	}

}

func (n *Node) StartListenIncomingMessages() {
	go func() {
		for {
			msg := <-n.msgChannel
			n.processIncomingMessage(msg)
			fmt.Println("msg received")
		}
	}()
}

func (n *Node) broadcastMessageToPeers(rawMsg []byte) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	for _, peer := range n.requestPeers {
		err := peer.SendMsg(rawMsg)
		if err != nil {
			peer.strikes += 1
		} else {
			peer.strikes -= 1
		}

	}
}

func (r *RequestPeer) SendMsg(msg []byte) error {
	secureRequest := ""

	host := r.ip
	if net.ParseIP(host) == nil {
		secureRequest = "s"
	}

	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http%s://%s/msg", secureRequest, r.ip), bytes.NewReader(msg))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		err = errors.New("bad request")
		return err

	}

	return nil
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
	case TxnMsg:
		n.ingressTxn <- msg.Txn
		fmt.Println("txnmsg")
	case BlockMsg:
		n.ingressBlock <- msg.Block
		fmt.Println("blkmsg")
	}

}

func (n *Node) BalancePeers(hostNames []string) {

	var currentPeers []string

	n.mutex.Lock()
	for peer := range n.requestPeers {
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

			// do nothing
			// eventually balance peers

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

func (n *Node) requestPeerBlock(ctx context.Context, ipAddress string, path string, height int, hash string) (Block, error) {

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
		return Block{}, err
	}

	secureRequest := ""

	host := ipAddress
	if net.ParseIP(host) == nil {
		secureRequest = "s"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http%s://%s/%s", secureRequest, ipAddress, path), bytes.NewReader(payloadData))
	if err != nil {
		return Block{}, err
	}

	fmt.Println("request made ")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Block{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		err = errors.New("bad request")
		return Block{}, err

	}

	var block Block

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		err = readErr
		return Block{}, err
	}

	fmt.Println("body read")

	err = json.Unmarshal(bodyBytes, &block)
	if err != nil {
		fmt.Println("Failed to decode JSON. Response was:")
		fmt.Println(string(bodyBytes))
		return Block{}, err
	}

	fmt.Println("block here: ", block)

	return block, nil
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

	secureRequest := ""

	host := ipAddress
	if net.ParseIP(host) == nil {
		secureRequest = "s"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http%s://%s/highest_block", secureRequest, ipAddress), nil)
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

	fmt.Println("starting peer sample")
	sample := n.GetAddressSample()
	if len(sample) == 0 {
		return "", 0, errors.New(NO_PEERS_ERROR)
	}

	fmt.Println("peer sample", sample)

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
	fmt.Println("racing peers")
	ctx := context.Background()

	var block Block
	var found bool

	sample := n.GetAddressSample()
	for _, address := range sample {
		fmt.Println("requestingblock from: ", address)
		fmt.Println("height", height)
		fmt.Println("hash", hash)
		// we cant "race" because our CPU would go nuts hashing argons
		foundBlock, err := n.requestPeerBlock(ctx, address, "block_get", height, hash)
		if err == nil {

			fmt.Println("Block found")
			fmt.Println(foundBlock)

			serialized := foundBlock.Serialize()
			if crypto.Hash(serialized) == hash {
				block = foundBlock
				found = true
				break
			}
		}

		fmt.Println("block find err", err)

	}

	if found {
		return block, nil
	} else {
		return Block{}, errors.New("no block found, need to resample")
	}
}

func (n *Node) RunEgress() {

	go func() {
		for {
			select {
			case txn := <-n.egressTxn:
				txnMsg := Message{
					Action: TxnMsg,
					Txn:    txn,
				}

				msg, err := json.Marshal(txnMsg)
				if err != nil {
					log.Fatal("peer broadcast: invalid slice")
				}
				n.broadcastMessageToPeers(msg)

			case block := <-n.egressBlock:
				blockMsg := Message{
					Action: BlockMsg,
					Block:  block,
				}

				msg, err := json.Marshal(blockMsg)
				if err != nil {
					log.Fatal("peer broadcast: invalid slice")
				}
				n.broadcastMessageToPeers(msg)

			}
		}

	}()
}

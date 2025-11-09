package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/canavan-a/broom/node/netnode"
)

const (
	secure   = "https://"
	insecure = "http://"
)

type Miner struct {
	MyPayoutAddress string

	PoolAddress string
	Protocol    string
	Workers     int

	done     chan struct{}
	solution chan netnode.Block
	signal   chan struct{}

	winTarget   string
	proofTarget string

	miningBlock netnode.Block

	LocalProofs int
	ProofsMutex sync.Mutex

	SlaveMode bool
}

func NewMiner(myPayoutAddress string, poolAddress string, workers int, slave bool) *Miner {

	var protocol string
	if net.ParseIP(poolAddress) == nil {
		protocol = insecure
	}
	protocol = secure

	miner := &Miner{
		MyPayoutAddress: myPayoutAddress,
		PoolAddress:     poolAddress,
		Protocol:        protocol,
		Workers:         workers,

		done:     make(chan struct{}),
		solution: make(chan netnode.Block),
		signal:   make(chan struct{}),

		miningBlock: netnode.Block{},

		ProofsMutex: sync.Mutex{},
		SlaveMode:   slave,
	}

	// test the connection to the specified pool
	version, err := RequestPool[any, string](miner, nil, Get, "version")
	if err != nil {
		panic(err)
	}

	fmt.Println("Pool is on node version ", version)

	return miner
}

func (m *Miner) Start() {

	if !m.GetPoolData() {
		panic("could not get initial data")
	}

	go m.RunMiningLoop()

	go m.PollPoolData()

	fmt.Println("mining pool started")

	select {}
}

func (m *Miner) GetPoolData() (changed bool) {

	proofTarget, err := RequestPool[any, string](m, nil, Get, "proof_target")
	if err != nil {
		fmt.Println("bad request for proof target")
		return
	}
	solutionTarget, err := RequestPool[any, string](m, nil, Get, "difficulty")
	if err != nil {
		fmt.Println("bad request for solution target")
		return
	}
	block, err := RequestPool[any, netnode.Block](m, nil, Get, "mining_block")
	if err != nil {
		fmt.Println("bad request for mining block")
		return
	}

	if m.proofTarget != proofTarget {
		fmt.Println(m.proofTarget, proofTarget)
		m.proofTarget = proofTarget
		changed = true
	}

	if m.winTarget != solutionTarget {
		fmt.Println(m.winTarget, solutionTarget)
		m.winTarget = solutionTarget
		changed = true
	}

	if m.miningBlock.Height != block.Height {
		fmt.Println("height diff")
		m.miningBlock = block
		changed = true
		return
	}

	if m.miningBlock.PreviousBlockHash != block.PreviousBlockHash {
		fmt.Println("hash diff")
		m.miningBlock = block
		changed = true
		return
	}

	if len(m.miningBlock.Transactions) != len(block.Transactions) {
		fmt.Println("txn diff")
		m.miningBlock = block
		changed = true
		return
	}
	return false
}

func (m *Miner) PollPoolData() {
	for {

		if m.GetPoolData() {
			m.signal <- struct{}{}
			m.ProofsMutex.Lock()
			m.LocalProofs = 0
			m.ProofsMutex.Unlock()
		}
		time.Sleep(2 * time.Second)
	}

}

func (m *Miner) RunMiningLoop() {
	RestartMineAction(m, func() {})
	for {
		select {
		case <-m.signal:
			RestartMineAction(m, func() {
				fmt.Println("new pool information, just restarting miner")
				fmt.Println("mining block: ", m.miningBlock.Height)
			})
		case sol := <-m.solution:
			fmt.Println(sol)
			RestartMineAction(m, func() {
				fmt.Println("you found a solution, tell the pool operator")
				_, err := RequestPool[netnode.Block, string](m, sol, Post, "block")
				if err != nil {
					fmt.Println("solution could not publish")
				}
			})
		}
	}
}

func RestartMineAction(m *Miner, action func()) {
	close(m.done)
	action()
	m.done = make(chan struct{})
	m.Mine()

}

func (m *Miner) Mine() {

	targetOperators := make(map[string]func(b netnode.Block))
	if !m.SlaveMode { // slave mode does not report proofs
		targetOperators[m.proofTarget] = m.ReportProof
	}
	fmt.Println("Mining started")
	m.miningBlock.MineWithWorkers(context.Background(), m.winTarget, m.Workers, m.solution, m.done, targetOperators)
}

func (m *Miner) ReportProof(b netnode.Block) {
	go func() {
		workProof := netnode.WorkProof{Address: m.MyPayoutAddress, Block: b}
		_, err := RequestPool[netnode.WorkProof, string](m, workProof, Post, "proof")
		if err != nil {
			fmt.Println(err)
			fmt.Println("proof could not publish")
			return
		}
		m.ProofsMutex.Lock()
		m.LocalProofs += 1

		fmt.Printf("\rcurrent block proofs: %v", m.LocalProofs)

		m.ProofsMutex.Unlock()

	}()
}

type RequestType string

const (
	Get  RequestType = "GET"
	Post RequestType = "POST"
)

func RequestPool[T, U any](m *Miner, payload T, requestType RequestType, path string) (response U, err error) {

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	fullUrl := fmt.Sprintf("%s%s", m.Url(), path)

	req, err := http.NewRequest(string(requestType), fullUrl, bytes.NewReader(data))
	if err != nil {
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode >= 299 {
		err = errors.New("bad request")
		return
	}

	defer res.Body.Close()

	resData, err := io.ReadAll(res.Body)
	if err != nil {
		return
	}

	switch any(response).(type) {
	case string:
		response = any(string(resData)).(U)
	default:
		err = json.Unmarshal(resData, &response)
		if err != nil {
			fmt.Println("error unmarshalling")

			return
		}
	}

	return

}

func (m *Miner) Url() string {
	return fmt.Sprintf("%s%s", m.Protocol, m.PoolAddress)
}

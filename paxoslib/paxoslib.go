package paxoslib

import (
	"cs.ubc.ca/cpsc416/project/util"
	"fmt"
	"github.com/DistributedClocks/tracing"
	"net"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type PaxosRPC Paxos

type Paxos struct {
	Id             uint8
	GlobalState    State
	LocalState     State
	SeqNum         int
	Acceptor       *Acceptor
	Proposer       *Proposer
	Learner        *Learner
	localMutex     sync.RWMutex
	globalMutex    sync.RWMutex
	tracer         *tracing.Tracer
	ptrace         *tracing.Trace
	failedServerCh chan Failed
}

func NewPaxos() *Paxos {
	return &Paxos{}
}

func (p *Paxos) Init(id uint8, serverAddr string, tracer *tracing.Tracer, failedServerCh chan Failed) error {

	p.Id = id
	p.failedServerCh = failedServerCh

	err := rpc.Register((*PaxosRPC)(p))
	if err != nil {
		util.Log(p.Id, "PaxosLib", "InitializeState", fmt.Sprintf("Error registering Paxos RPC: %s", err))
		return err
	}

	err = p.setupRPCServer(serverAddr)
	if err != nil {
		util.Log(p.Id, "PaxosLib", "InitializeState", fmt.Sprintf("Error setting up Paxos RPC server: %s", err))
		return err
	}

	//initialize mutexes

	p.SeqNum = 0
	p.tracer = tracer
	p.ptrace = tracer.CreateTrace()

	p.ptrace.RecordAction(PaxosInit{p.Id})

	return nil
}

// InitializeState only allowed to call once
func (p *Paxos) InitializeState(paxosListenAddrList []string) {

	util.Log(p.Id, "PaxosLib", "InitializeState", fmt.Sprintf("🪵 Paxoslib initializing state (# of servers = %d, # of max failures = %d)",
		len(paxosListenAddrList), (len(paxosListenAddrList)-1)/2))

	// Create State with all servers
	p.GlobalState = State{
		Servers: make(map[uint8]Server),
	}

	p.LocalState = State{
		Servers: make(map[uint8]Server),
	}

	for i := 1; i <= len(paxosListenAddrList); i++ {
		localServer := Server{
			Id:      uint8(i),
			IsAlive: true,
		}

		globalServer := Server{
			Id:      uint8(i),
			IsAlive: true,
		}

		rpcClient, err := rpc.Dial("tcp", paxosListenAddrList[i-1])
		localServer.client = rpcClient
		globalServer.client = rpcClient
		if err != nil {
			util.Log(p.Id, "PaxosLib", "InitializeState", fmt.Sprintf("Error rpc.Dial on server %d: %s", i, err))
			return
		}

		p.LocalState.Servers[uint8(i)] = localServer
		p.GlobalState.Servers[uint8(i)] = globalServer
	}

	p.Proposer = NewProposer(p.Id, p)
	p.Acceptor = NewAcceptor(p.Id, p)
	p.Learner = NewLearner(p.Id, p)

	p.ptrace.RecordAction(InitializeState{p.Id})
}

//From A3
func (p *Paxos) setupRPCServer(addr string) error {
	tcpServerListenAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		util.Log(p.Id, "PaxosLib", "setupRPCServer", fmt.Sprintf("Error resolving TCP address: %s", err))
	}

	serverListener, err := net.ListenTCP("tcp", tcpServerListenAddr)
	if err != nil {
		util.Log(p.Id, "PaxosLib", "setupRPCServer", fmt.Sprintf("Error listening on TCP address: %s", err))
	}

	go rpc.Accept(serverListener)

	util.Log(p.Id, "PaxosLib", "setupRPCServer", "RPC server setup for Paxos")

	return nil
}

func (p *Paxos) countLeadingLiveServers() int {
	count := 0
	for _, server := range p.GetGlobalState().Servers {
		if server.Id == p.Id {
			break
		}
		if server.IsAlive {
			count++
		}
	}
	return count
}

// UpdateCurrentState Fcheck found a failure
func (p *Paxos) UpdateCurrentState(failedServer Failed, token tracing.TracingToken) bool {
	util.Log(p.Id, "PaxosLib", "UpdateCurrentState", fmt.Sprintf("🗳 <gid= %d> Asking Paxos to confirm failure of server: %d", failedServer.GId, failedServer.ServerId))

	localServer := p.LocalState.Servers[failedServer.ServerId]

	localServer.IsAlive = false

	p.LocalState.Servers[failedServer.ServerId] = localServer

	//update local state
	time.Sleep(time.Millisecond * 200 * time.Duration(p.countLeadingLiveServers()))
	go p.runAlgorithm(p.LocalState, token, failedServer.GId)

	// sleep for 10 seconds
	time.Sleep(10 * time.Second)

	if p.GlobalState.Servers[failedServer.ServerId].IsAlive {
		localServer.IsAlive = true
		p.LocalState.Servers[failedServer.ServerId] = localServer
		return false
	}
	return true
}

func (p *Paxos) GetGlobalState() State {
	p.globalMutex.RLock()
	defer p.globalMutex.RUnlock()
	return p.GlobalState
}

//// ValidateState client wants to know GlobalState
//func (p *Paxos) ValidateState() {
//	p.runAlgorithm(p.GetLocalState())
//}

func (p *Paxos) GetLocalState() State {
	return p.LocalState
}

func (p *Paxos) runAlgorithm(s State, token tracing.TracingToken, GId uint) {
	p.Proposer.Prepare(s, token, GId)
}

func (p *Paxos) SendPrepare(m Message) {

	trace := p.tracer.ReceiveToken(m.Token)
	trace.RecordAction(PrepareReq{m.From, m.SeqNum, m.GId})
	token := trace.GenerateToken()
	// loop over all servers in p.GlobalState
	var promises []Promise
	for _, server := range p.GlobalState.Servers {

		if server.IsAlive {

			message := Message{
				From:   m.From,
				To:     server.Id,
				SeqNum: m.SeqNum,
				Token:  token,
				GId:    m.GId,
			}

			promise := Promise{}
			done := make(chan *rpc.Call, 1)
			util.Log(p.Id, "PaxosLib", "SendPrepare", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Prepare 🤞", m.SeqNum, m.GId, server.Id))

			server.client.Go("PaxosRPC.ReceivePrepare", message, &promise, done)

			select {
			case r := <-done:
				if r.Error != nil {
					util.Log(p.Id, "PaxosLib", "SendPrepare", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Prepare ERROR 🤞❌", m.SeqNum, m.GId, server.Id))
					continue
				}
			case <-time.After(time.Millisecond * 300):
				util.Log(p.Id, "PaxosLib", "SendPrepare", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Prepare TIMEOUT 🤞⏳", m.SeqNum, m.GId, server.Id))
				continue
			}

			promises = append(promises, promise)
		}
	}
	p.Proposer.ReceivePromise(m, promises)
}

func (p *Paxos) SendPropose(m Message) {

	go func() {
		res := Message{}
		done := make(chan *rpc.Call, 1)
		util.Log(p.Id, "PaxosLib", "SendPropose", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Propose 💍", m.SeqNum, m.GId, m.To))

		p.GlobalState.Servers[m.To].client.Go("PaxosRPC.ReceivePropose", m, &res, done)

		select {
		case r := <-done:
			if r.Error != nil {
				util.Log(p.Id, "PaxosLib", "SendPropose", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Propose ERROR 💍❌", m.SeqNum, m.GId, m.To))
			}
		}
	}()
}

func (p *Paxos) SendBroadcast(m Message) {

	trace := p.tracer.ReceiveToken(m.Token)
	trace.RecordAction(BroadcastReq{p.Id, m.SeqNum, m.GId})
	token := trace.GenerateToken()

	for _, server := range p.GlobalState.Servers {

		if server.IsAlive {
			internalServer := server

			go func() {
				message := Message{
					ProposedState: m.ProposedState,
					From:          p.Id,
					To:            internalServer.Id,
					SeqNum:        m.SeqNum,
					Token:         token,
					GId:           m.GId,
				}

				res := Message{}

				done := make(chan *rpc.Call, 1)
				util.Log(p.Id, "PaxosLib", "SendBroadcast", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Broadcast 📣", m.SeqNum, m.GId, internalServer.Id))

				internalServer.client.Go("PaxosRPC.ReceiveBroadcast", message, &res, done)

				select {
				case r := <-done:
					if r.Error != nil {
						util.Log(p.Id, "PaxosLib", "SendBroadcast", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Broadcast ERROR 📣❌", m.SeqNum, m.GId, internalServer.Id))
					}
				case <-time.After(time.Millisecond * 300):
					util.Log(p.Id, "PaxosLib", "SendBroadcast", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Broadcast TIMEOUT 📣⏳", m.SeqNum, m.GId, internalServer.Id))
				}
			}()
		}
	}
}

func (p *Paxos) setLearnedState(m Message) {

	p.globalMutex.Lock()
	defer p.globalMutex.Unlock()

	trace := p.tracer.ReceiveToken(m.Token)
	trace.RecordAction(Learn{p.Id, m.ProposedState.Servers, m.GId})
	token := trace.GenerateToken()

	go func() {
		time.Sleep(1 * time.Second)
		util.Log(p.Id, "PaxosLib", "setLearnedState", fmt.Sprintf("<pid= %d, gid= %d> ✨✨✨ Learned New State: %v ✨✨✨", m.SeqNum, m.GId, FormatServerState(m.ProposedState)))
	}()

	for _, server := range m.ProposedState.Servers {
		globalServer := p.GlobalState.Servers[server.Id]
		localServer := p.LocalState.Servers[server.Id]

		if globalServer.IsAlive != server.IsAlive && !server.IsAlive {
			go func() { p.failedServerCh <- Failed{ServerId: globalServer.Id, GId: m.GId} }()

			go func() {
				time.Sleep(1 * time.Second)
				done := make(chan *rpc.Call, 1)
				message := Message{
					From:   p.Id,
					To:     globalServer.Id,
					SeqNum: m.SeqNum,
					GId:    m.GId,
					Token:  token,
				}
				res := Message{}

				globalServer.client.Go("PaxosRPC.Kill", message, &res, done)
				select {
				case <-done:
				case <-time.After(time.Millisecond * 300):
				}
			}()

		}

		globalServer.IsAlive = server.IsAlive
		p.GlobalState.Servers[server.Id] = globalServer

		localServer.IsAlive = server.IsAlive
		p.LocalState.Servers[server.Id] = localServer
	}

	// If I'm the dead server, then exit
	if !p.GlobalState.Servers[p.Id].IsAlive {
		util.Log(p.Id, "PaxosLib", "setLearnedState", fmt.Sprintf("<pid= %d, gid= %d> 💣💣💣 Exiting (I'm the dead server) 💣💣💣", m.SeqNum, m.GId))
		os.Exit(0)
	}

	// Count the number of dead servers
	numDeadServers := 0
	for _, server := range p.GlobalState.Servers {
		if !server.IsAlive {
			numDeadServers++
		}
	}

	// If number of dead servers is greater than half of the total number of servers, then exit
	quorum := len(p.GlobalState.Servers) / 2
	if numDeadServers >= quorum {
		trace.RecordAction(MajorityDead{m.SeqNum, m.GId})
		util.Log(p.Id, "PaxosLib", "setLearnedState", fmt.Sprintf("<pid= %d, gid= %d> 💣⏳💣 Majority - 1 Failures Detected (Dying in 20s) 💣⏳💣", m.SeqNum, m.GId))
		// Sleep for 20 seconds
		p.globalMutex.Unlock()
		time.Sleep(20 * time.Second)
		util.Log(p.Id, "PaxosLib", "setLearnedState", fmt.Sprintf("<pid= %d, gid= %d> 💣💣💣 Exiting (Majority Dead To Me) 💣💣💣", m.SeqNum, m.GId))
		trace.RecordAction(GracefulExit{m.SeqNum, m.GId})
		os.Exit(0)
	}
}

func (p *PaxosRPC) ReceivePrepare(m Message, r *Promise) error {
	trace := p.tracer.ReceiveToken(m.Token)
	trace.RecordAction(PrepareRecvd{m.From, m.To, m.SeqNum, m.GId})

	*r = p.Acceptor.ReceivePrepare(m)

	trace.RecordAction(PromiseReq{m.To, r.SeqNum, m.GId})
	r.Token = trace.GenerateToken()

	return nil
}

func (p *PaxosRPC) ReceivePropose(m Message, r *Message) error {

	p.Acceptor.ReceivePropose(m)

	*r = Message{
		From:          m.To,
		To:            m.From,
		SeqNum:        m.SeqNum,
		ProposedState: m.ProposedState,
		GId:           m.GId,
	}
	return nil
}

func (p *PaxosRPC) ReceiveBroadcast(m Message, r *Message) error {
	trace := p.tracer.ReceiveToken(m.Token)
	trace.RecordAction(BroadcastRecvd{m.From, m.To, m.SeqNum, m.GId})
	m.Token = trace.GenerateToken()

	p.Learner.ReceiveAccept(m)

	return nil
}

// Kill RPC call to kill server
func (p *PaxosRPC) Kill(m Message, r *Message) error {
	if m.To != p.Id {
		util.Log(p.Id, "PaxosLib", "Kill", fmt.Sprintf("<pid= %d, gid= %d> [<-- %d] Kill not intended for me (it's for %d)", m.SeqNum, m.GId, m.From, m.To))
	} else {
		trace := p.tracer.ReceiveToken(m.Token)
		trace.RecordAction(KillRecvd{m.From, m.To, m.SeqNum, m.GId})
		util.Log(p.Id, "PaxosLib", "Kill", fmt.Sprintf("<pid= %d, gid= %d> [<-- %d] 💣💣💣 Exiting (Been asked to die) 💣💣💣", m.SeqNum, m.GId, m.From))
		os.Exit(0)
	}
	return nil
}

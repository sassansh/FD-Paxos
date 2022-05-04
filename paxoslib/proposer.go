package paxoslib

import (
	"cs.ubc.ca/cpsc416/project/util"
	"fmt"
	"github.com/DistributedClocks/tracing"
	"math/rand"
	"time"
)

type Proposer struct {
	id          uint8
	seqNum      int
	paxosLib    *Paxos
	numRejected int
}

func NewProposer(id uint8, paxosLib *Paxos) *Proposer {
	rand.Seed(time.Now().UnixNano())
	return &Proposer{id, 0, paxosLib, 0}
}

//Prepare Update the sequence number and send a prepare message to all acceptors
// Sending to all acceptors is necessary in case of a network partition
func (p *Proposer) Prepare(s State, token tracing.TracingToken, GId uint) {
	p.seqNum++

	message := Message{
		ProposedState: s,
		From:          p.id,
		SeqNum:        p.seqNum,
		Token:         token,
		GId:           GId,
	}

	globalState := p.paxosLib.GetGlobalState()

	diffState := false

	//Verify whether the local state differs from the global state.
	// If it differs, send prepare messages
	for _, server := range s.Servers {
		globalServer := globalState.Servers[server.Id]
		if globalServer.IsAlive != server.IsAlive {
			diffState = true
			p.paxosLib.SendPrepare(message)
			break
		}
	}

	//Do not send a prepare message if the state is the same
	if !diffState {
		go func() {
			time.Sleep(time.Millisecond * 1100)
			trace := p.paxosLib.tracer.ReceiveToken(token)
			trace.RecordAction(PrepareAborted{p.seqNum, GId})
			util.Log(p.id, "Proposer", "Prepare", fmt.Sprintf("<pid= %d, gid= %d> 🛑 No change in state, stopping next prepare", p.seqNum, GId))
		}()

	}
}

//ReceivePromise Receive promises from acceptors
func (p *Proposer) ReceivePromise(m Message, promises []Promise) {
	quorum := len(p.paxosLib.GetGlobalState().Servers)/2 + 1
	totalPromised := 0
	var promisedIDs []uint8

	largestSeqNum := m.SeqNum

	var trace *tracing.Trace
	//Check whether the acceptor has sent a promise
	for _, promise := range promises {
		trace = p.paxosLib.tracer.ReceiveToken(promise.Token)
		trace.RecordAction(PromiseRecvd{promise.From, promise.To, promise.SeqNum, promise.GId})
		if promise.Promised {
			util.Log(p.id, "Proposer", "ReceivePromise", fmt.Sprintf("<pid= %d, gid= %d> [<-- %d] Promise 🤞", m.SeqNum, m.GId, promise.From))
			totalPromised++
			promisedIDs = append(promisedIDs, promise.From)
		} else {
			//If the acceptor did not send a promise, store the largest sequence number seen
			if promise.MySeqNum > largestSeqNum {
				largestSeqNum = promise.MySeqNum
			}
		}
	}

	//If the majority of acceptors have sent a promise, send them proposals
	if totalPromised >= quorum {
		util.Log(p.id, "Proposer", "ReceivePromise", fmt.Sprintf("<pid= %d, gid= %d> 🎉 Received Majority Promises 🤞", m.SeqNum, m.GId))
		m.Token = trace.GenerateToken()
		p.Propose(m, promisedIDs)
		p.numRejected = 0
	} else {
		//If there is not a majority of promises, wait using random exponential backoff, update the sequence number, and send another prepare
		waitTime := ((2^p.numRejected)*100 + rand.Intn((300-25+1)+25)) * 4
		util.Log(p.id, "Proposer", "ReceivePromise", fmt.Sprintf("<pid= %d, gid= %d> 🚨 Contention detected (Waiting %dms)", m.SeqNum, m.GId, waitTime))
		if largestSeqNum > p.seqNum {
			p.seqNum = largestSeqNum
		}
		time.Sleep(time.Millisecond * time.Duration(waitTime))
		p.numRejected++
		p.Prepare(m.ProposedState, m.Token, m.GId)
	}
}

// Propose Send a proposal to all acceptors that returned a promise
func (p *Proposer) Propose(m Message, promisedIDs []uint8) {
	util.Log(p.id, "Proposer", "Propose", fmt.Sprintf("<pid= %d, gid= %d> 🎉💍 Propose State: %s", m.SeqNum, m.GId, FormatServerState(m.ProposedState)))

	trace := p.paxosLib.tracer.ReceiveToken(m.Token)
	trace.RecordAction(ProposeReq{p.id, m.SeqNum, m.GId})
	token := trace.GenerateToken()

	for _, promisedID := range promisedIDs {
		proposeMessage := Message{
			ProposedState: m.ProposedState,
			SeqNum:        m.SeqNum,
			From:          p.id,
			To:            promisedID,
			Token:         token,
			GId:           m.GId,
		}

		p.paxosLib.SendPropose(proposeMessage)
	}
}

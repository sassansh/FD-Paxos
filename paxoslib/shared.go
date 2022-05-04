package paxoslib

import (
	"fmt"
	"github.com/DistributedClocks/tracing"
	"net/rpc"
)

type Failed struct {
	ServerId uint8
	GId      uint
}

type State struct {
	Servers map[uint8]Server
}

type Server struct {
	IsAlive bool
	Id      uint8
	client  *rpc.Client
}

type Message struct {
	ProposedState State
	SeqNum        int
	From          uint8
	To            uint8
	Token         tracing.TracingToken
	GId           uint
}

type Promise struct {
	SeqNum   int
	From     uint8
	To       uint8
	Promised bool
	MySeqNum int
	Token    tracing.TracingToken
	GId      uint
}

type FailedServers struct {
	Failed []Failed
	Token  tracing.TracingToken
}

func FormatServerState(state State) string {

	serversFormatted := "["

	for serverId := uint8(1); serverId <= uint8(len(state.Servers)); serverId++ {
		serversFormatted += fmt.Sprintf("{%d: ", serverId)
		if state.Servers[serverId].IsAlive {
			serversFormatted += fmt.Sprintf("Alive}")
		} else {
			serversFormatted += fmt.Sprintf("Dead}")
		}
		// if next server exists
		if serverId < uint8(len(state.Servers)) {
			serversFormatted += ", "
		}
	}

	serversFormatted += "]"

	return serversFormatted
}

// Tracing structs
// ----------------------------------------------------------------------------------------

type PaxosInit struct {
	ServerId uint8
}

type InitializeState struct {
	ServerId uint8
}

type PrepareReq struct {
	From uint8
	PSN  int
	GId  uint
}

type PrepareRecvd struct {
	From uint8
	To   uint8
	PSN  int
	GId  uint
}

type PromiseReq struct {
	From uint8
	PSN  int
	GId  uint
}

type PromiseRecvd struct {
	From uint8
	To   uint8
	PSN  int
	GId  uint
}

type ProposeReq struct {
	From uint8
	PSN  int
	GId  uint
}

type ProposeRecvd struct {
	From uint8
	To   uint8
	PSN  int
	GId  uint
}

type BroadcastReq struct {
	From uint8
	PSN  int
	GId  uint
}

type BroadcastRecvd struct {
	From uint8
	To   uint8
	PSN  int
	GId  uint
}

type ProposalChosen struct {
	ServerId uint8
	PSN      int
	GId      uint
}

type ProposalRejected struct {
	ServerId uint8
	PSN      int
	GId      uint
}

type Learn struct {
	ServerId uint8
	Servers  map[uint8]Server
	GId      uint
}

type GracefulExit struct {
	PSN int
	GId uint
}

type MajorityDead struct {
	PSN int
	GId uint
}

type KillRecvd struct {
	From uint8
	To   uint8
	PSN  int
	GId  uint
}

type PrepareAborted struct {
	PSN int
	GId uint
}

// ----------------------------------------------------------------------------------------

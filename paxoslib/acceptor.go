package paxoslib

import (
	"cs.ubc.ca/cpsc416/project/util"
	"fmt"
)

func NewAcceptor(id uint8, paxosLib *Paxos) *Acceptor {
	return &Acceptor{
		id:          id,
		paxosLib:    paxosLib,
		lastPromise: Message{SeqNum: 0},
	}
}

type Acceptor struct {
	id          uint8
	paxosLib    *Paxos
	lastPromise Message
}

// ReceivePrepare receives prepare messages from proposers and responds with an
// promise message if the prepare Id is greater than the last accepted.
func (a *Acceptor) ReceivePrepare(prepare Message) Promise {
	if prepare.SeqNum > a.lastPromise.SeqNum {
		util.Log(a.id, "Acceptor", "ReceivePrepare", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Promise SENT 🤞✅", prepare.SeqNum, prepare.GId, prepare.From))
		a.lastPromise = prepare
		return Promise{
			SeqNum:   prepare.SeqNum,
			From:     a.id,
			To:       prepare.From,
			Promised: true,
			Token:    prepare.Token,
			GId:      prepare.GId,
		}
	} else {
		util.Log(a.id, "Acceptor", "ReceivePrepare", fmt.Sprintf("<pid= %d, gid= %d> [--> %d] Promise NOT SENT 🤞❌", prepare.SeqNum, prepare.GId, prepare.From))
		return Promise{
			SeqNum:   prepare.SeqNum,
			MySeqNum: a.lastPromise.SeqNum,
			From:     a.id,
			To:       prepare.From,
			Promised: false,
			Token:    prepare.Token,
			GId:      prepare.GId,
		}
	}
}

// Acceptor receives propose messages from proposers and responds with an
// accept message if the propose message is greater than the last accepted.
func (a *Acceptor) ReceivePropose(proposeMsg Message) {
	sameState := true
	currentState := a.paxosLib.GetLocalState()

	trace := a.paxosLib.tracer.ReceiveToken(proposeMsg.Token)
	trace.RecordAction(ProposeRecvd{proposeMsg.From, proposeMsg.To, proposeMsg.SeqNum, proposeMsg.GId})

	for _, server := range proposeMsg.ProposedState.Servers {
		// Find same server Id in current GlobalState
		if server.IsAlive != currentState.Servers[server.Id].IsAlive {
			sameState = false
			break
		}
	}

	if a.lastPromise.SeqNum == proposeMsg.SeqNum && sameState {
		trace.RecordAction(ProposalChosen{proposeMsg.To, proposeMsg.SeqNum, proposeMsg.GId})
		util.Log(a.id, "Acceptor", "ReceivePropose", fmt.Sprintf("<pid= %d, gid= %d> [<-- %d] Propose ACCEPTED 💍✅", proposeMsg.SeqNum, proposeMsg.GId, proposeMsg.From))
		acceptMessage := Message{
			ProposedState: proposeMsg.ProposedState,
			SeqNum:        proposeMsg.SeqNum,
			From:          a.id,
			Token:         trace.GenerateToken(),
			GId:           proposeMsg.GId,
		}
		a.paxosLib.SendBroadcast(acceptMessage)
	} else {
		trace.RecordAction(ProposalRejected{proposeMsg.To, proposeMsg.SeqNum, proposeMsg.GId})
		util.Log(a.id, "Acceptor", "ReceivePropose", fmt.Sprintf("<pid= %d, gid= %d> [<-- %d] Propose REJECTED 💍❌", proposeMsg.SeqNum, proposeMsg.GId, proposeMsg.From))
	}
}

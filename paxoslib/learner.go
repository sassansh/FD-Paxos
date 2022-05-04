package paxoslib

import (
	"cs.ubc.ca/cpsc416/project/util"
	"fmt"
)

func NewLearner(id uint8, paxosLib *Paxos) *Learner {
	// Count the number of acceptors
	numAcceptors := len(paxosLib.GlobalState.Servers)
	quorum := ((numAcceptors) / 2) + 1

	return &Learner{id: id,
		quorumCount:      quorum,
		seqNumMsgMap:     make(map[int]Message),
		seqNumMap:        make(map[int]int),
		seqNumLearnerMap: make(map[uint8]int),
		paxosLib:         paxosLib,
	}
}

type Learner struct {
	id               uint8
	paxosLib         *Paxos
	quorumCount      int
	seqNumMsgMap     map[int]Message
	seqNumMap        map[int]int
	seqNumLearnerMap map[uint8]int
}

func (l *Learner) ReceiveAccept(acceptMsg Message) {
	lastSeqNumFromAcceptor := l.seqNumLearnerMap[acceptMsg.From]

	if lastSeqNumFromAcceptor < acceptMsg.SeqNum {
		util.Log(l.id, "Learner", "ReceiveAccept", fmt.Sprintf("<pid= %d, gid= %d> [<-- %d] Broadcast ACCEPTED 📣✅", acceptMsg.SeqNum, acceptMsg.GId, acceptMsg.From))
		// Update the last seq num from acceptor
		l.seqNumLearnerMap[acceptMsg.From] = acceptMsg.SeqNum

		// Store the message in the map
		l.seqNumMsgMap[acceptMsg.SeqNum] = acceptMsg

		// Update the count of messages received for this SeqNum
		l.seqNumMap[acceptMsg.SeqNum]++

		// If we have received quorumCount messages for this SeqNum, then we can update the system state
		if l.seqNumMap[acceptMsg.SeqNum] == l.quorumCount {
			util.Log(l.id, "Learner", "ReceiveAccept", fmt.Sprintf("<pid= %d, gid= %d> 🎊 Received Majority Broadcasts 📣", acceptMsg.SeqNum, acceptMsg.GId))
			l.paxosLib.setLearnedState(l.seqNumMsgMap[acceptMsg.SeqNum])
		}
	} else {
		util.Log(l.id, "Learner", "ReceiveAccept", fmt.Sprintf("<pid= %d, gid= %d> [<-- %d] Broadcast REJECTED 📣❌", acceptMsg.SeqNum, acceptMsg.GId, acceptMsg.From))
	}
}

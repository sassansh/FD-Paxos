package paxos

import (
	fchecker "cs.ubc.ca/cpsc416/project/fcheck"
	"cs.ubc.ca/cpsc416/project/paxoslib"
	"cs.ubc.ca/cpsc416/project/util"
	"fmt"
	"github.com/DistributedClocks/tracing"
	"math/rand"
	"net"
	"net/rpc"
	"time"
)

// Tracing structs
// ----------------------------------------------------------------------------------------

type ServerStart struct {
	ServerId uint8
}

type ServerJoining struct {
	ServerId uint8
}

type ServerJoined struct {
	ServerId uint8
}

type AllServersJoined struct {
	ServerId uint8
}

type ServerFailureDetected struct {
	FailedServerId uint8
	GId            uint
}

type GetFailedServersReqRecvd struct {
}

type GetFailedServersRes struct {
	FailedServers []paxoslib.Failed /* List of failed servers */
}

type ServerFailureNotVerified struct {
	FailedServerId uint8
	GId            uint
}

type ServerRPC Server

// ----------------------------------------------------------------------------------------

type ServerConfig struct {
	ServerId             uint8
	TracingServerAddr    string
	Secret               []byte
	TracingIdentity      string
	ClientListenAddrList []string
	ServerListenAddrList []string
	FCheckServerAddrList []string
	PaxosListenAddrList  []string
	NumServers           int
}

type Server struct {
	serverId             uint8
	clientListenAddr     string
	serverListenAddr     string
	fCheckServerAddr     string
	paxosListenAddr      string
	tracingServerAddr    string
	clientListenAddrList []string
	serverListenAddrList []string
	fCheckServerAddrList []string
	paxosListenAddrList  []string
	numServers           int
	GId                  uint
	tracer               *tracing.Tracer
	strace               *tracing.Trace
	paxos                *paxoslib.Paxos
}

func NewServer() *Server {
	return &Server{
		serverId:             0,
		clientListenAddr:     "",
		serverListenAddr:     "",
		fCheckServerAddr:     "",
		tracingServerAddr:    "",
		clientListenAddrList: make([]string, 0),
		serverListenAddrList: make([]string, 0),
		fCheckServerAddrList: make([]string, 0),
		numServers:           0,
	}
}

func (s *Server) Start(tracer *tracing.Tracer, serverId uint8, clientListenAddrList []string, serverListenAddrList []string, fcheckServerAddrList []string, paxosListenAddrList []string, numServers int) error {
	s.tracer = tracer
	s.tracer.SetShouldPrint(false)
	s.strace = tracer.CreateTrace()
	s.serverId = serverId
	s.serverListenAddr = serverListenAddrList[s.serverId-1]
	s.clientListenAddr = clientListenAddrList[s.serverId-1]
	s.fCheckServerAddr = fcheckServerAddrList[s.serverId-1]
	s.paxosListenAddr = paxosListenAddrList[s.serverId-1]
	s.numServers = numServers
	s.paxosListenAddrList = paxosListenAddrList[:numServers]
	s.clientListenAddrList = clientListenAddrList[:numServers]
	s.serverListenAddrList = serverListenAddrList[:numServers]
	s.fCheckServerAddrList = fcheckServerAddrList[:numServers]

	s.GId = uint(s.serverId)

	util.Log(serverId, "Server", "Start", fmt.Sprintf("🔄 Starting server with ID %d", s.serverId))
	util.Log(serverId, "Server", "Start", fmt.Sprintf("Expecting %d servers to join...", numServers))
	s.strace.RecordAction(ServerStart{s.serverId})

	// Initialize fcheck
	notifyCh, err := fchecker.Initialize()
	if err != nil {
		util.Log(serverId, "Server", "Start", fmt.Sprintf("Error initializing fcheck: %s", err))
		return err
	}

	// Start responding to heartbeats
	fchecker.StartAcknowledging(s.fCheckServerAddr)
	util.Log(serverId, "Fcheck", "StartAcknowledging", fmt.Sprintf("🫀 Responding to heartbeats now"))

	// Register server's RPC
	rpc.Register((*ServerRPC)(s))
	serverListenTCPAddr, err := net.ResolveTCPAddr("tcp", s.serverListenAddr)
	if err != nil {
		util.Log(serverId, "Server", "Start", fmt.Sprintf("Failed to resolve TCP address %s", err))
		return err
	}
	serverListener, err := net.ListenTCP("tcp", serverListenTCPAddr)
	if err != nil {
		util.Log(serverId, "Server", "Start", fmt.Sprintf("Failed to listen on TCP address %s", err))
		return err
	}
	go rpc.Accept(serverListener)
	util.Log(serverId, "Server", "Start", fmt.Sprintf("RPC server setup for server joining"))

	// Initialize Paxos
	failedServerCh := make(chan paxoslib.Failed, 1)
	s.paxos = paxoslib.NewPaxos()
	err = s.paxos.Init(s.serverId, s.paxosListenAddr, s.tracer, failedServerCh)
	go s.removeFailedServersFromFcheck(failedServerCh)
	if err != nil {
		util.Log(serverId, "Server", "Start", fmt.Sprintf("Failed to initialize paxos %s", err))
		return err
	}

	// Start the joining process
	s.strace.RecordAction(ServerJoining{ServerId: s.serverId})
	err = s.joinServers()
	if err != nil {
		util.Log(serverId, "Server", "Start", fmt.Sprintf("Failed to join servers: %s", err))
		return err
	}
	util.Log(serverId, "Server", "JoinComplete", fmt.Sprintf("🏁 All %d servers joined", numServers))
	s.strace.RecordAction(AllServersJoined{s.serverId})

	// Initialize the Paxos state
	s.paxos.InitializeState(s.paxosListenAddrList)

	// Start failure detection
	go s.detectFailures(notifyCh)

	// Open the client listener
	clientListenTCPAddr, err := net.ResolveTCPAddr("tcp", s.clientListenAddr)
	if err != nil {
		util.Log(serverId, "Server", "Start", fmt.Sprintf("Failed to resolve clientListenAddr %s", err))
		return err
	}
	clientListener, err := net.ListenTCP("tcp", clientListenTCPAddr)
	if err != nil {
		util.Log(serverId, "Server", "Start", fmt.Sprintf("Failed to listen on clientListenAddr %s", err))
		return err
	}

	// Server is now ready to accept requests
	time.Sleep(time.Second)

	util.Log(serverId, "Server", "Start", fmt.Sprintf("✅ System is LIVE & ready to accept client requests!"))

	rpc.Accept(clientListener)

	return nil
}

func (s *ServerRPC) GetFailedServers(req paxoslib.FailedServers, res *paxoslib.FailedServers) error {
	// Do tracing
	ctrace := s.tracer.ReceiveToken(req.Token)
	ctrace.RecordAction(GetFailedServersReqRecvd{})

	// Ask Paxos for state
	stateFromPaxos := s.paxos.GetGlobalState()

	// Prepare response of failed servers
	failedServers := paxoslib.FailedServers{}
	for _, server := range stateFromPaxos.Servers {
		if server.IsAlive == false {
			failedServers.Failed = append(failedServers.Failed, paxoslib.Failed{
				ServerId: server.Id,
			})
		}
	}

	// Set response
	ctrace.RecordAction(GetFailedServersRes{failedServers.Failed})
	res.Failed = failedServers.Failed
	res.Token = ctrace.GenerateToken()

	return nil
}

func (s *Server) detectFailures(notifyCh chan fchecker.FailureDetected) error {
	// Separate the IP addresses from the listen addresses
	fcheckIP, _, _ := net.SplitHostPort(s.serverListenAddr)
	fcheckIP = fcheckIP + ":0"

	// Start to monitor all other servers
	for i := 1; i <= s.numServers; i++ {
		if uint8(i) != s.serverId {
			err := fchecker.AddNodeToMonitor(fchecker.MonitorNodeStruct{EpochNonce: uint64(rand.Int63()),
				HBeatLocalIPHBeatLocalPort: fcheckIP, HBeatRemoteIPHBeatRemotePort: s.fCheckServerAddrList[i-1],
				LostMsgThresh: 10})
			if err != nil {
				util.Log(s.serverId, "Server", "detectFailures", fmt.Sprintf("Failed to add node to monitor %s", err))
			}
		}
	}

	// Start listening for failure notifications
	for {
		notification := <-notifyCh

		// Find serverId of server that failed
		failedServerId := uint8(0)
		for i := 1; i <= s.numServers; i++ {
			if s.fCheckServerAddrList[i-1] == notification.UDPIpPort {
				failedServerId = uint8(i)
			}
		}
		s.GId += 1000
		currentGId := s.GId
		util.Log(s.serverId, "Server", "Fcheck", fmt.Sprintf("<gid= %d> Detected failed server: %d", currentGId, failedServerId))
		s.strace.RecordAction(ServerFailureDetected{failedServerId, currentGId})

		// Ask Paxos to verify the failure
		go func() {
			success := s.paxos.UpdateCurrentState(paxoslib.Failed{ServerId: failedServerId, GId: currentGId}, s.strace.GenerateToken())
			if !success {
				// Failure not verified, add server back to fcheck
				util.Log(s.serverId, "Server", "Fcheck", fmt.Sprintf("⛔️ Paxos run did not complete for failed server: %d", failedServerId))
				s.strace.RecordAction(ServerFailureNotVerified{failedServerId, currentGId})
				err := fchecker.AddNodeToMonitor(fchecker.MonitorNodeStruct{EpochNonce: uint64(rand.Int63()),
					HBeatLocalIPHBeatLocalPort: fcheckIP, HBeatRemoteIPHBeatRemotePort: s.fCheckServerAddrList[failedServerId-1],
					LostMsgThresh: 10})
				if err != nil {
					util.Log(s.serverId, "Server", "detectFailures", fmt.Sprintf("Failed to add node to monitor %s", err))
				}
			}
		}()
	}
}

func (s *ServerRPC) Join(req uint8, res *uint8) error {
	// RPC call to join the system
	*res = s.serverId
	return nil
}

func (s *Server) joinServers() error {
	util.Log(s.serverId, "Server", "ServerJoining", fmt.Sprintf("Server %d joined", s.serverId))
	currentServerId := 1
	for currentServerId <= s.numServers {
		if uint8(currentServerId) == s.serverId {
			currentServerId++
			continue
		}
		laddr, _ := net.ResolveTCPAddr("tcp", ":0")
		raddr, _ := net.ResolveTCPAddr("tcp", s.serverListenAddrList[currentServerId-1])
		conn, err := net.DialTCP("tcp", laddr, raddr)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		serverConn := rpc.NewClient(conn)

		var res uint8
		err = serverConn.Call("ServerRPC.Join", s.serverId, &res)
		if err != nil {
			util.Log(s.serverId, "Server", "ServerJoining", fmt.Sprintf("Error calling ServerRPC.Join on %d", currentServerId))
			return err
		}
		conn.SetLinger(0)
		conn.Close()
		serverConn.Close()
		util.Log(s.serverId, "Server", "ServerJoining", fmt.Sprintf("Server %d joined", currentServerId))
		currentServerId++
	}

	s.strace.RecordAction(ServerJoined{s.serverId})

	return nil
}

func (s *Server) removeFailedServersFromFcheck(failedServersCh chan paxoslib.Failed) {
	for {
		failedServer := <-failedServersCh
		// Paxos has verified a failure, remove the server from fcheck
		fchecker.StopMonitoringNode(s.fCheckServerAddrList[failedServer.ServerId-1])
	}
}

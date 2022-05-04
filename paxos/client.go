package paxos

import (
	"cs.ubc.ca/cpsc416/project/paxoslib"
	"errors"
	"github.com/DistributedClocks/tracing"
	"log"
	"net"
	"net/rpc"
	"time"
)

type ClientConfig struct {
	ClientID          string
	ServerAddrList    []string
	TracingServerAddr string
	Secret            []byte
	TracingIdentity   string
}

// Tracing structs
// ----------------------------------------------------------------------------------------

type ClientStart struct {
}

type NewQueryServer struct {
	ServerAddress string /* ip:port */
	ServerId      int    /* Implementation assumes ServerIds are 1 plus the indices of ClientConfig ServerAddrList */
}

type ConnectionError struct {
	ServerAddress string
	ServerId      int
}

type AllServersDown struct {
}

type GetFailedServersReq struct {
}

type GetFailedServersResRecvd struct {
	FailedServers []paxoslib.Failed /* List of failed servers */
}

// ----------------------------------------------------------------------------------------

type Client struct {
	clientID         string            /* Currently unused; Keeping in case we extend support to multiple clients */
	fdServer         *rpc.Client       /* Server to query for failed server list */
	currentServerPos int               /* Index of the current server */
	serverCount      int               /* Number of available servers */
	failCount        int               /* Number of servers that have timed out or returned errors (from clients perspective) */
	timeoutTime      time.Duration     /* Number of seconds that must elapse for a timeout to be triggered */
	serverAddrs      []string          /* List of all FD server addresses */
	paxosFailures    []paxoslib.Failed /* List of failed servers */
	ctrace           *tracing.Trace    /* Client trace */
}

const timeoutError string = "fdServerTimeout"
const allServersDown string = "could not reach any fdServers "

/* Client constructor */
func NewClient(tracer *tracing.Tracer, clientId string, ServerAddrList []string) *Client {
	if len(ServerAddrList) == 0 {
		log.Fatal("no server addresses supplied to client")
	}
	return &Client{
		clientID:         clientId,
		fdServer:         nil,
		currentServerPos: 0,
		serverCount:      len(ServerAddrList),
		failCount:        0,
		timeoutTime:      time.Second * 5.0,
		serverAddrs:      ServerAddrList,
		ctrace:           tracer.CreateTrace(),
	}
}

/* Start initial connection; Called on first query for failed servers */
func (c *Client) start() (err error) {
	if c.fdServer != nil {
		return nil
	}
	c.ctrace.RecordAction(ClientStart{})
	serverAddr := c.serverAddrs[c.currentServerPos]
	c.fdServer, err = connectToServer(serverAddr)
	if err != nil {
		c.ctrace.RecordAction(ConnectionError{serverAddr, c.currentServerPos + 1})
	} else {
		c.ctrace.RecordAction(NewQueryServer{serverAddr, c.currentServerPos + 1})
	}
	return err
}

/* Close connection to server if it exists */
func (c *Client) CloseConnection() {
	if c.fdServer != nil {
		c.fdServer.Close()
		c.fdServer = nil
	}
}

/* Get a list of failed servers according to Paxos */
func (c *Client) GetSystemState(tracer *tracing.Tracer) ([]paxoslib.Failed, error) {
	validReply := false
	c.failCount = 0 // Reset the failed server count
	var res *paxoslib.FailedServers

	// Get server connection
	server, err := c.getCurrentServer()
	if err != nil {
		server, err = c.getNextServer()
		if err != nil && err.Error() == allServersDown {
			return nil, err
		}
	}

	// Query server
	for !validReply {
		c.ctrace.RecordAction(GetFailedServersReq{})
		req := paxoslib.FailedServers{Token: c.ctrace.GenerateToken()}
		res, err = c.sendMessage(server, "ServerRPC.GetFailedServers", req, c.timeoutTime)
		if err != nil {
			c.ctrace.RecordAction(ConnectionError{ServerAddress: c.serverAddrs[c.currentServerPos], ServerId: c.currentServerPos + 1})
			// Try next server
			server, err = c.getNextServer()
			if err != nil && err.Error() == allServersDown {
				return nil, err
			}
		} else {
			validReply = true
		}
	}

	// Trace and write result to client
	tracer.ReceiveToken(res.Token)
	c.ctrace.RecordAction(GetFailedServersResRecvd{res.Failed})

	// Update stored list of failures and return
	c.paxosFailures = res.Failed
	return res.Failed, err
}

/* Call server RPC method with timeout */
func (c *Client) sendMessage(conn *rpc.Client, serviceMethod string, req paxoslib.FailedServers, timeout time.Duration) (res *paxoslib.FailedServers, err error) {
	pending := conn.Go(serviceMethod, req, &res, make(chan *rpc.Call, 1))
	for {
		select {
		case call := <-pending.Done:
			return res, call.Error
		case <-time.After(timeout):
			return nil, errors.New(timeoutError)
		}
	}
}

/* Create an RPC connection to remoteIPPort */
func connectToServer(remoteIPPort string) (*rpc.Client, error) {
	remoteAddr, err := net.ResolveTCPAddr("tcp", remoteIPPort)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTCP("tcp", nil, remoteAddr)
	if err != nil {
		log.Println("Client: Error connecting to", remoteIPPort, ": ", err)
		return nil, err
	}
	conn.SetLinger(0)
	return rpc.NewClient(conn), err
}

/* Get current server connection */
func (c *Client) getCurrentServer() (server *rpc.Client, err error) {
	if c.fdServer == nil {
		err = c.start()
	}
	return c.fdServer, err
}

/* Get a rpc.Client connection to the next server */
func (c *Client) getNextServer() (server *rpc.Client, err error) {
	serverAddr, err := c.getNextServerAddress()
	if err != nil {
		c.ctrace.RecordAction(AllServersDown{}) // All servers cannot be reached
		return nil, err
	}
	c.fdServer, err = connectToServer(serverAddr)

	for err != nil {
		c.ctrace.RecordAction(ConnectionError{serverAddr, c.currentServerPos + 1})
		serverAddr, err = c.getNextServerAddress()
		if err != nil {
			c.ctrace.RecordAction(AllServersDown{}) // All servers cannot be reached
			return nil, err
		}
		c.fdServer, err = connectToServer(serverAddr)
	}
	c.ctrace.RecordAction(NewQueryServer{serverAddr, c.currentServerPos + 1})
	return c.fdServer, err
}

/* Get the next server address */
func (c *Client) getNextServerAddress() (string, error) {
	// Check if all servers are unreachable
	c.failCount++
	if c.failCount >= c.serverCount {
		return "", errors.New(allServersDown)
	}

	// Increment server index
	c.currentServerPos++
	c.currentServerPos = c.currentServerPos % c.serverCount

	// Check if the new index is the same as a failed server. If so, skip it and increment failCount.
	// Note, this implementation assumes that at least one server is not in the paxosFailures list
	validServer := false
	for !validServer {
		validServer = true
		for _, failed := range c.paxosFailures {
			if failed.ServerId == uint8(c.currentServerPos+1) {
				c.failCount++
				if c.failCount >= c.serverCount {
					return "", errors.New(allServersDown)
				}
				c.currentServerPos++
				c.currentServerPos = c.currentServerPos % c.serverCount
				validServer = false
				break
			}
		}
	}
	return c.serverAddrs[c.currentServerPos], nil
}

package main

import (
	"cs.ubc.ca/cpsc416/project/paxos"
	"cs.ubc.ca/cpsc416/project/util"
	"github.com/DistributedClocks/tracing"
)

func main() {
	var config paxos.ServerConfig
	util.ReadJSONConfig("cloud-config/server10_config.json", &config)
	stracer := tracing.NewTracer(tracing.TracerConfig{
		ServerAddress:  config.TracingServerAddr,
		TracerIdentity: config.TracingIdentity,
		Secret:         config.Secret,
	})
	server := paxos.NewServer()
	server.Start(stracer, config.ServerId, config.ClientListenAddrList, config.ServerListenAddrList, config.FCheckServerAddrList, config.PaxosListenAddrList, config.NumServers)
}

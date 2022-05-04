package main

import (
	"cs.ubc.ca/cpsc416/project/paxoslib"
	"fmt"
	"github.com/DistributedClocks/tracing"
	"sort"
	"time"

	paxos "cs.ubc.ca/cpsc416/project/paxos"
	"cs.ubc.ca/cpsc416/project/util"
)

func main() {
	var config paxos.ClientConfig
	err := util.ReadJSONConfig("cloud-config/client_config.json", &config)
	util.CheckErr(err, "Error reading client config: %v\n", err)

	tracer := tracing.NewTracer(tracing.TracerConfig{
		ServerAddress:  config.TracingServerAddr,
		TracerIdentity: config.TracingIdentity,
		Secret:         config.Secret,
	})

	globalDeadServerIds := ""

	tracer.SetShouldPrint(false)

	client := paxos.NewClient(tracer, config.ClientID, config.ServerAddrList)
	first := true
	var failures []paxoslib.Failed
	i := 0
	for err == nil {
		failures, err = client.GetSystemState(tracer)
		if first || !first && len(failures) > 0 {
			first = false

			// convert dead server id to string
			deadServerIds := "["

			// sort the dead servers
			sort.Slice(failures, func(i, j int) bool {
				return failures[i].ServerId < failures[j].ServerId
			})

			for i, v := range failures {
				deadServerIds += fmt.Sprintf("%d", v.ServerId)

				// if more dead servers exists
				if i+1 < len(failures) {
					deadServerIds += ", "
				}
			}

			deadServerIds += "]"

			if globalDeadServerIds != deadServerIds {
				globalDeadServerIds = deadServerIds
				util.Log(0, "Client", "GetSystemState", fmt.Sprintf("🪦 Detected Failures: %s", deadServerIds))
			}
		}

		time.Sleep(time.Second * 2.0)
		i++
		if i == 10 {
			client.CloseConnection()
			client.CloseConnection()
		}
	}
	util.CheckErr(err, "Error getting list of failed servers %v\b", err)
}

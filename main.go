package main

import (
	"context"
	"flag"
	"os"

	"github.com/miekg/dns"
	"github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/log"
	"github.com/osrg/gobgp/v3/pkg/server"
)

var peerGroupName = "default-peer-group"
var logger = log.NewDefaultLogger()

func main() {
	configPath := flag.String("c", "config.yaml", "config path")
	flag.Parse()

	configFile, err := os.Open(*configPath)
	if err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}
	defer configFile.Close()

	config, err := ReadConfig(configFile)
	if err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}

	s := server.NewBgpServer(server.LoggerOption(logger))
	go s.Serve()

	if err := s.StartBgp(context.Background(), &apipb.StartBgpRequest{Global: &apipb.Global{
		Asn:        config.BGP.ASN,
		RouterId:   config.BGP.RouterID,
		ListenPort: int32(config.BGP.ListenPort),
	}}); err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}

	r := &apipb.WatchEventRequest{Peer: &apipb.WatchEventRequest_Peer{}}
	if err := s.WatchEvent(context.Background(), r, func(r *apipb.WatchEventResponse) {
		if p := r.GetPeer(); p != nil && p.Type == apipb.WatchEventResponse_PeerEvent_STATE {
			logger.Info(p.String(), log.Fields{})
		}
	}); err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}

	if err := s.AddPeerGroup(context.Background(), &apipb.AddPeerGroupRequest{
		PeerGroup: &apipb.PeerGroup{
			Conf: &apipb.PeerGroupConf{
				PeerAsn:       config.BGP.Neighbors.ASN,
				PeerGroupName: peerGroupName,
			},
		},
	}); err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}

	if err := s.AddDynamicNeighbor(context.Background(), &apipb.AddDynamicNeighborRequest{
		DynamicNeighbor: &apipb.DynamicNeighbor{
			Prefix:    config.BGP.Neighbors.Prefix,
			PeerGroup: peerGroupName,
		},
	}); err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}

	autoBGP, err := NewAutoBGP(s, config.DNS.AuthoritativeDNS, config.Rules)
	if err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}

	if err := dns.ListenAndServe(config.DNS.Listen, config.DNS.Network, autoBGP); err != nil {
		logger.Fatal(err.Error(), log.Fields{})
	}
}

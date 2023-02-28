package main

import (
	"context"
	"flag"
	"os"

	"github.com/miekg/dns"
	apipb "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/log"
	"github.com/osrg/gobgp/v3/pkg/server"
	"google.golang.org/protobuf/types/known/anypb"
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

	if err := s.StartBgp(context.Background(), &apipb.StartBgpRequest{
		Global: &apipb.Global{
			Asn:        config.BGP.ASN,
			RouterId:   config.BGP.RouterID,
			ListenPort: int32(config.BGP.ListenPort),
		},
	}); err != nil {
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

type AutoBGP struct {
	dnsClient dns.Client
	dnsConn   *dns.Conn

	bgpServer *server.BgpServer

	netxHopAttr *anypb.Any
	originAttr  *anypb.Any

	rules RulesConfig
}

func NewAutoBGP(bgpServer *server.BgpServer, authoritativeDNS string, rules RulesConfig) (*AutoBGP, error) {
	autoBGP := &AutoBGP{
		dnsClient: dns.Client{Net: "tcp-tls"},
		bgpServer: bgpServer,
		rules:     rules,
	}

	netxHopAttr, err := anypb.New(&apipb.NextHopAttribute{NextHop: rules.NextHop})
	if err != nil {
		return nil, err
	}
	autoBGP.netxHopAttr = netxHopAttr

	conn, err := autoBGP.dnsClient.DialContext(context.Background(), authoritativeDNS)
	if err != nil {
		return nil, err
	}
	autoBGP.dnsConn = conn

	originAttr, err := anypb.New(&apipb.OriginAttribute{Origin: 0})
	if err != nil {
		return nil, err
	}
	autoBGP.originAttr = originAttr

	return autoBGP, nil
}

func (a *AutoBGP) Close() {
	a.dnsConn.Close()
}

func (a *AutoBGP) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	defer w.Close()

	resp, err := a.handleMessage(req)
	if err != nil {
		logger.Error(err.Error(), log.Fields{})
		return
	}

	if err := w.WriteMsg(resp); err != nil {
		logger.Error(err.Error(), log.Fields{})
		return
	}
}

func (a *AutoBGP) handleMessage(req *dns.Msg) (*dns.Msg, error) {
	resp, _, err := a.dnsClient.ExchangeWithConn(req, a.dnsConn)
	if err != nil {
		return nil, err
	}

	for _, answer := range resp.Answer {
		// A records are currently only supported
		answer, ok := answer.(*dns.A)
		if !ok {
			continue
		}

		if match := a.rules.Zones.Matches(answer.Hdr.Name); match == "" {
			continue
		}

		if len(answer.A) == 0 || answer.A.IsUnspecified() || answer.A.IsLoopback() || answer.A.IsPrivate() {
			continue
		}

		ip := apipb.IPAddressPrefix{
			Prefix:    answer.A.String(),
			PrefixLen: 32,
		}

		nlri, err := anypb.New(&ip)
		if err != nil {
			return nil, err
		}

		_, err = a.bgpServer.AddPath(context.Background(), &apipb.AddPathRequest{
			TableType: apipb.TableType_ADJ_OUT,
			Path: &apipb.Path{
				Nlri:   nlri,
				Pattrs: []*anypb.Any{a.originAttr, a.netxHopAttr},
				Family: &apipb.Family{Afi: apipb.Family_AFI_IP, Safi: apipb.Family_SAFI_UNICAST},
			},
		})
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

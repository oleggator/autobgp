package main

import (
	"context"
	"github.com/miekg/dns"
	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	apb "google.golang.org/protobuf/types/known/anypb"
	"log"
)

var (
	nextHop = "192.168.1.1"

	zones = Zones{
		"netflix.com.",
	}

	asn       uint32 = 65432
	routerID         = "192.168.88.247"
	bgpListen int32  = 179

	neighborAddress        = "192.168.88.1"
	neighborAsn     uint32 = 64512

	authoritativeDNS = "1.1.1.1:853"
)

func init() {
	zones.Normalize()
}

func main() {
	s := server.NewBgpServer()
	go s.Serve()

	if err := s.StartBgp(context.Background(), &api.StartBgpRequest{
		Global: &api.Global{
			Asn:        asn,
			RouterId:   routerID,
			ListenPort: bgpListen,
		},
	}); err != nil {
		log.Fatal(err)
	}

	r := &api.WatchEventRequest{Peer: &api.WatchEventRequest_Peer{}}
	if err := s.WatchEvent(context.Background(), r, func(r *api.WatchEventResponse) {
		if p := r.GetPeer(); p != nil && p.Type == api.WatchEventResponse_PeerEvent_STATE {
			log.Println(p)
		}
	}); err != nil {
		log.Fatal(err)
	}

	if err := s.AddPeer(context.Background(), &api.AddPeerRequest{
		Peer: &api.Peer{
			Conf: &api.PeerConf{
				NeighborAddress: neighborAddress,
				PeerAsn:         neighborAsn,
			},
		},
	}); err != nil {
		log.Fatal(err)
	}

	autoBGP, err := NewAutoBGP(s, nextHop)
	if err != nil {
		log.Fatalln(err)
	}

	if err := dns.ListenAndServe("0.0.0.0:53", "udp", autoBGP); err != nil {
		log.Fatalln(err)
	}
}

type AutoBGP struct {
	dnsClient dns.Client
	dnsConn   *dns.Conn

	bgpServer *server.BgpServer

	netxHopAttr *apb.Any
	originAttr  *apb.Any
}

func NewAutoBGP(bgpServer *server.BgpServer, nextHop string) (*AutoBGP, error) {
	autoBGP := &AutoBGP{
		dnsClient: dns.Client{Net: "tcp-tls"},
		bgpServer: bgpServer,
	}

	netxHopAttr, err := apb.New(&api.NextHopAttribute{NextHop: nextHop})
	if err != nil {
		return nil, err
	}
	autoBGP.netxHopAttr = netxHopAttr

	conn, err := autoBGP.dnsClient.DialContext(context.Background(), authoritativeDNS)
	if err != nil {
		return nil, err
	}
	autoBGP.dnsConn = conn

	originAttr, err := apb.New(&api.OriginAttribute{Origin: 0})
	if err != nil {
		return nil, err
	}
	autoBGP.originAttr = originAttr

	return autoBGP, nil
}

func (a *AutoBGP) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	defer w.Close()

	resp, _, err := a.dnsClient.ExchangeWithConn(req, a.dnsConn)
	if err != nil {
		log.Fatalln(err)
	}

	for _, answer := range resp.Answer {
		answer, ok := answer.(*dns.A)
		if !ok {
			continue
		}

		if match := zones.Matches(answer.Header().Name); match == "" {
			continue
		}

		ip := api.IPAddressPrefix{
			Prefix:    answer.A.String(),
			PrefixLen: 32,
		}
		log.Println(answer.Header().Name, ip.Prefix)

		nlri, err := apb.New(&ip)
		if err != nil {
			log.Fatalln(err)
		}

		_, err = a.bgpServer.AddPath(context.Background(), &api.AddPathRequest{
			TableType: api.TableType_ADJ_OUT,
			Path: &api.Path{
				Nlri:   nlri,
				Pattrs: []*apb.Any{a.originAttr, a.netxHopAttr},
				Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
			},
		})
		if err != nil {
			log.Fatalln(err)
		}
	}

	if err := w.WriteMsg(resp); err != nil {
		log.Fatalln(err)
	}
}

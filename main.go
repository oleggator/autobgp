package main

import (
	"context"
	"github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	apb "google.golang.org/protobuf/types/known/anypb"
	"log"
)

func main() {
	s := server.NewBgpServer()
	go s.Serve()

	if err := s.StartBgp(context.Background(), &api.StartBgpRequest{
		Global: &api.Global{
			Asn:              65432,
			RouterId:         "192.168.88.247",
			ListenPort:       179,
			UseMultiplePaths: true,
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

	family := &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}
	n := &api.Peer{
		Conf: &api.PeerConf{
			NeighborAddress: "192.168.88.1",
			PeerAsn:         64512,
		},
	}

	if err := s.AddPeer(context.Background(), &api.AddPeerRequest{
		Peer: n,
	}); err != nil {
		log.Fatal(err)
	}

	addr := "127.0.0.1:6000"
	if err := listen(addr, func(message *dnstap.Message) {
		if *message.Type == dnstap.Message_CLIENT_RESPONSE {
			msg := &dns.Msg{}
			if err := msg.Unpack(message.ResponseMessage); err != nil {
				log.Fatalln(err)
			}

			for _, answer := range msg.Answer {
				switch answer := answer.(type) {
				case *dns.A:
					a1, _ := apb.New(&api.OriginAttribute{
						Origin: 0,
					})
					a2, _ := apb.New(&api.NextHopAttribute{
						NextHop: "192.168.1.1",
					})

					nlri, _ := apb.New(&api.IPAddressPrefix{
						Prefix:    answer.A.String(),
						PrefixLen: 32,
					})
					_, err := s.AddPath(context.Background(), &api.AddPathRequest{
						TableType: api.TableType_ADJ_OUT,
						Path: &api.Path{
							Nlri:   nlri,
							Pattrs: []*apb.Any{a1, a2},
							Family: family,
						},
					})
					if err != nil {
						log.Fatalln(err)
					}
				}
			}
		}
	}); err != nil {
		log.Fatalln(err)
	}
}

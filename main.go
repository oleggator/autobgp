package main

import (
	"context"
	"github.com/coredns/coredns/plugin"
	"github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
	api "github.com/osrg/gobgp/v3/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	apb "google.golang.org/protobuf/types/known/anypb"
	"log"
)

var dnstapListen = "127.0.0.1:6000"
var gobgpGRPCAddr = "127.0.0.1:50051"
var nextHop = "192.168.1.1"
var zones = plugin.Zones{
	"netflix.com.",
}

func init() {
	zones.Normalize()
}

func main() {
	conn, err := grpc.DialContext(context.TODO(), gobgpGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalln(err)
	}
	s := NewGoBGPAPI(conn)

	dnstapHandler := func(message *dnstap.Message) {
		if *message.Type != dnstap.Message_CLIENT_RESPONSE {
			return
		}

		var msg dns.Msg
		if err := msg.Unpack(message.ResponseMessage); err != nil {
			log.Fatalln(err)
		}

		for _, answer := range msg.Answer {
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
			if err := s.AddRoute(context.Background(), &ip, nextHop); err != nil {
				log.Fatalln(err)
			}
		}
	}
	if err := listen(dnstapListen, dnstapHandler); err != nil {
		log.Fatalln(err)
	}
}

type GoBGPAPI struct {
	s api.GobgpApiClient
}

func NewGoBGPAPI(conn grpc.ClientConnInterface) GoBGPAPI {
	return GoBGPAPI{s: api.NewGobgpApiClient(conn)}
}

func (a *GoBGPAPI) AddRoute(ctx context.Context, ip *api.IPAddressPrefix, nextHop string) error {
	originAttr, err := apb.New(&api.OriginAttribute{Origin: 0})
	if err != nil {
		return err
	}

	netxHopAttr, err := apb.New(&api.NextHopAttribute{NextHop: nextHop})
	if err != nil {
		return err
	}

	nlri, err := apb.New(ip)
	if err != nil {
		return err
	}

	_, err = a.s.AddPath(ctx, &api.AddPathRequest{
		TableType: api.TableType_ADJ_OUT,
		Path: &api.Path{
			Nlri:   nlri,
			Pattrs: []*apb.Any{originAttr, netxHopAttr},
			Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		},
	})
	if err != nil {
		return err
	}

	return nil
}

package main

import (
	dnstap "github.com/dnstap/golang-dnstap"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
)

func listen(addr string, handler func(message *dnstap.Message)) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	ch := make(chan []byte)
	defer close(ch)

	go func(ch <-chan []byte) {
		for frame := range ch {
			var dt dnstap.Dnstap
			if err := proto.Unmarshal(frame, &dt); err != nil {
				log.Fatalln(err)
			}

			handler(dt.Message)
		}
	}(ch)

	dnstap.NewFrameStreamSockInput(l).ReadInto(ch)

	return err
}

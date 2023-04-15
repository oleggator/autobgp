package main

import (
	"github.com/miekg/dns"
	"time"
)

type DNSClient struct {
	client *dns.Client
	conn   *dns.Conn

	authoritativeDNS string
	retries          int
}

func NewDNSClient(authoritativeDNS string) (DNSClient, error) {
	client := &dns.Client{Net: "tcp-tls", Timeout: 10 * time.Second}

	return DNSClient{
		client:           client,
		authoritativeDNS: authoritativeDNS,
		retries:          5,
	}, nil
}

func (c *DNSClient) ExchangeDNS(m *dns.Msg) (r *dns.Msg, rtt time.Duration, err error) {
	for i := 0; i < c.retries; i++ {
		if c.conn == nil {
			if c.conn, err = c.client.Dial(c.authoritativeDNS); err != nil {
				return nil, 0, err
			}
		}

		r, rtt, err = c.client.ExchangeWithConn(m, c.conn)
		if err != nil {
			if c.conn != nil {
				c.conn.Close()
			}

			c.conn = nil

			continue
		}

		return r, rtt, nil
	}

	return nil, 0, err
}

package netrelay

import (
	"context"
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/txthinking/socks5"
)

func ExampleClient_tcp() {
	c, err := socks5.NewClient("127.0.0.1:9999", "", "", 0, 60)
	if err != nil {
		panic(err)
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return c.Dial(network, addr)
			},
		},
	}
	res, err := client.Get("https://ifconfig.co")
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	log.Println(string(b))
	// Output:
}

func ExampleClient_udp() {
	c, err := socks5.NewClient("127.0.0.1:9999", "", "", 0, 60)
	if err != nil {
		panic(err)
	}
	conn, err := c.Dial("udp", "8.8.8.8:53")
	if err != nil {
		panic(err)
	}
	b, err := hex.DecodeString("0001010000010000000000000a74787468696e6b696e6703636f6d0000010001")
	if err != nil {
		panic(err)
	}
	if _, err := conn.Write(b); err != nil {
		panic(err)
	}
	b = make([]byte, 2048)
	n, err := conn.Read(b)
	if err != nil {
		panic(err)
	}
	b = b[:n]
	b = b[len(b)-4:]
	log.Println(net.IPv4(b[0], b[1], b[2], b[3]))
	// Output:
}

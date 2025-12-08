package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"
)

type UpstreamResolver struct {
	upstreamAddr string
	timeout      time.Duration
}

func NewUpstreamResolver(upstreamAddr string) *UpstreamResolver {
	return &UpstreamResolver{
		upstreamAddr: upstreamAddr,
		timeout:      5 * time.Second,
	}
}

func (u *UpstreamResolver) Resolve(ctx context.Context, query []byte) ([]byte, error) {
	var (
		conn      net.Conn
		err       error
		deadline  time.Time
		response  []byte = make([]byte, 512)
		bytesRead int
	)
	conn, err = net.Dial("udp", u.upstreamAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to upstream: %w", err)
	}
	defer conn.Close()

	deadline = time.Now().Add(u.timeout)
	conn.SetDeadline(deadline)

	if _, err = conn.Write(query); err != nil {
		return nil, fmt.Errorf("failed to write query: %w", err)
	}

	bytesRead, err = conn.Read(response)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	response = bytes.Clone(response[:bytesRead])
	return response, nil
}

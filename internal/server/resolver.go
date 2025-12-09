package server

import (
	"bytes"
	"context"
	"flash-dns/internal/logger"
	"fmt"
	"net"
	"strings"
	"time"
)

type UpstreamResolver struct {
	upstreamAddrs []string
	timeout       time.Duration
}

func NewUpstreamResolver(upstream string) *UpstreamResolver {
	var addresses []string = strings.Split(upstream, ",")
	for i, v := range addresses {
		addresses[i] = strings.TrimSpace(v) + ":53"
	}

	return &UpstreamResolver{
		upstreamAddrs: addresses,
		timeout:       5 * time.Second,
	}
}

func (u *UpstreamResolver) Resolve(ctx context.Context, query []byte) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	var (
		queryCtx     context.Context
		cancel       context.CancelFunc
		response     []byte      = make([]byte, 512)
		responseChan chan []byte = make(chan []byte, len(u.upstreamAddrs))
	)
	queryCtx, cancel = context.WithCancel(ctx)
	for _, address := range u.upstreamAddrs {
		go u.resolveUpstream(queryCtx, address, query, responseChan)
	}

	select {
	case response = <-responseChan:
		cancel()
		return response, nil

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-time.After(u.timeout):
		return nil, fmt.Errorf("all upstream dns failed")
	}

}

func (u *UpstreamResolver) resolveUpstream(ctx context.Context, address string, query []byte, responseChan chan []byte) {
	var (
		conn      net.Conn
		err       error
		deadline  time.Time
		response  []byte = make([]byte, 512)
		bytesRead int
	)
	conn, err = net.Dial("udp", address)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to connect to upstream %s: %w", address, err))
		return
	}
	defer conn.Close()

	deadline = time.Now().Add(u.timeout)
	conn.SetDeadline(deadline)

	if _, err = conn.Write(query); err != nil {
		logger.Error(fmt.Sprintf("failed to write query to %s: %w", address, err))
		return
	}

	bytesRead, err = conn.Read(response)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to read response from %s: %w", address, err))
		return
	}

	select {
	case responseChan <- bytes.Clone(response[:bytesRead]):
	case <-ctx.Done():
		return
	}
}

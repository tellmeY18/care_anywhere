package main

import (
	"context"
	"net"
)

type machine interface {
	Dial(context.Context, string, string) (net.Conn, error)
	Done() <-chan struct{}
	Close() error
}

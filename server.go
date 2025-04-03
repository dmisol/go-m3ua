// Copyright 2018-2024 go-m3ua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package m3ua

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/ishidawataru/sctp"
)

// Listener is a M3UA listener.
type Listener struct {
	sctpListener *sctp.SCTPListener
	*Config
}

// Listen returns a M3UA listener.
func Listen(net string, laddr *sctp.SCTPAddr, cfg *Config) (*Listener, error) {
	var err error
	l := &Listener{Config: cfg}

	n, ok := netMap[net]
	if !ok {
		return nil, fmt.Errorf("invalid network: %s", net)
	}

	l.sctpListener, err = sctp.ListenSCTP(n, laddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen SCTP: %w", err)
	}
	return l, nil
}

// Accept waits for and returns the next connection to the listener.
// After successfully established the association with peer, Payload can be read with Read() func.
// Other signals are automatically handled background in another goroutine.
func (l *Listener) Accept(ctx context.Context, q chan *ServeEvent, id int) (*Conn, error) {
	defer fmt.Println("server accept done", id)
	conn := &Conn{
		mode:        modeServer,
		stateChan:   make(chan State),
		established: make(chan struct{}),
		sctpInfo:    &sctp.SndRcvInfo{PPID: 0x03000000, Stream: 0},
		cfg:         l.Config,

		serviceChan: q,
		id:          id,
	}

	if conn.cfg.HeartbeatInfo.Interval == 0 {
		conn.cfg.HeartbeatInfo.Enabled = false
	}

	c, err := l.sctpListener.Accept()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	var ok bool
	conn.sctpConn, ok = c.(*sctp.SCTPConn)
	if !ok {
		err = errors.New("failed to assert conn")
		fmt.Println("monitor accept", err)
		return nil, err
	}

	go func() {
		conn.stateChan <- StateAspDown
	}()

	fmt.Println("monitor conn is", c)
	go conn.monitor(ctx)
	select {
	case _, ok := <-conn.established:
		fmt.Println("established")
		if !ok {
			fmt.Println("not ok")
			conn.sctpConn.Close()
			return nil, ErrFailedToEstablish
		}
		return conn, nil
	case <-time.After(30 * time.Second):
		fmt.Println("srv accept TO")
		return nil, ErrTimeout
	}
}

// Close closes the listener.
func (l *Listener) Close() error {
	// XXX - should close on M3UA layer.
	return l.sctpListener.Close()
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	return l.sctpListener.Addr()
}

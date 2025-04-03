// Copyright 2018-2024 go-m3ua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package m3ua

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/dmisol/go-m3ua/messages"
	"github.com/dmisol/go-m3ua/messages/params"
)

// State represents ASP State.
type State uint8

// M3UA status definitions.
const (
	StateAspDown State = iota
	StateAspInactive
	StateAspActive
	StateSCTPCDI
	StateSCTPRI
)

func (c *Conn) handleStateUpdate(current State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.state
	c.state = current

	switch c.mode {
	case modeClient:
		if err := c.handleStateUpdateAsClient(current, previous); err != nil {
			fmt.Println("handleStateUpdateAsClient", err)
			return err
		}
		return nil
	case modeServer:
		if err := c.handleStateUpdateAsServer(current, previous); err != nil {
			fmt.Println("handleStateUpdateAsServer", err)
			return err
		}
		return nil
	default:
		return errors.New("not implemented yet")
	}
}

func (c *Conn) handleStateUpdateAsClient(current, previous State) error {
	// fmt.Println("handle:", current, previous)

	switch current {
	case StateAspDown:
		c.sctpInfo.Stream = 0
		return c.initiateASPSM()
	case StateAspInactive:
		// todo: ? re-design:
		// if (rk && !rc)) - send REGREQ
		// if rc - send AspAc
		if c.cfg.RoutingKey != nil && previous == StateAspDown {
			fmt.Println("issue ReqReq")
			return c.initiateREQREQ()
		}
		fmt.Println("issue AspAC")
		return c.initiateASPTM()
	case StateAspActive:
		if current != previous {
			c.established <- struct{}{}
			c.beatAllow.Broadcast()
		}

		// fixme!!!
		c.sctpInfo.Stream = 1

		return nil
	case StateSCTPCDI, StateSCTPRI:
		return ErrSCTPNotAlive
	default:
		return ErrInvalidState
	}
}

func (c *Conn) handleStateUpdateAsServer(current, previous State) error {
	switch current {
	case StateAspDown:
		// do nothing. just wait for the message from peer and state is updated
		return nil
	case StateAspInactive:
		// do nothing. just wait for the message from peer and state is updated
		// XXX - send DAVA to notify peer?
		return nil
	case StateAspActive:
		if current != previous {
			c.established <- struct{}{}
			c.beatAllow.Broadcast()
		}
		return nil
	case StateSCTPCDI, StateSCTPRI:
		return ErrSCTPNotAlive
	default:
		return ErrInvalidState
	}
}

func (c *Conn) handleSignals(ctx context.Context, m3 messages.M3UA) {
	//fmt.Println("sig:", c.state, m3.MessageClassName(), m3.MessageTypeName(), m3.MessageClass(), m3.MessageType())
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Signal validations
	if m3.Version() != 1 {
		c.errChan <- NewErrInvalidVersion(m3.Version())
		return
	}

	switch msg := m3.(type) {
	// Transfer message
	case *messages.Data:
		go c.handleData(ctx, msg)
		c.stateChan <- c.state
	// ASPSM
	case *messages.AspUp:
		if err := c.handleAspUp(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- StateAspInactive
	case *messages.AspUpAck:
		if err := c.handleAspUpAck(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- StateAspInactive
	case *messages.AspDown:
		if err := c.handleAspDown(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- StateAspDown
	case *messages.AspDownAck:
		if err := c.handleAspDownAck(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- StateAspDown
	// ASPTM
	case *messages.AspActive:
		if err := c.handleAspActive(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- StateAspActive
	case *messages.AspActiveAck:
		/*
			if err := c.handleAspActiveAck(msg); err != nil {
				// c.errChan <- err
			}
		*/
		s := &params.Param{}
		s.Tag = params.RegistrationResult
		s.Length = 8
		s.Data = []byte{0x0, 0xd, 0x0, 0x8, 0x0, 0x1, 0x0, 0x3}

		x := &params.Param{}
		x.Tag = params.RegistrationResult
		x.Length = 8
		x.Data = []byte{0x0, 0x6, 0x0, 0x8, 0x0, 0x0, 0x0, 0x6}

		/*
			if _, err := c.WriteSignal(
				messages.NewNotify(s, nil, x, nil),
			); err != nil {
				c.errChan <- err
				return
			}
		*/

		c.stateChan <- StateAspActive
	case *messages.AspInactive:
		if err := c.handleAspInactive(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- StateAspInactive
	case *messages.AspInactiveAck:
		if err := c.handleAspInactiveAck(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- StateAspInactive
	case *messages.Heartbeat:
		if err := c.handleHeartbeat(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- c.state
	case *messages.HeartbeatAck:
		if err := c.handleHeartbeatAck(msg); err != nil {
			c.errChan <- err
		}
		c.beatAckChan <- struct{}{}
		c.stateChan <- c.state
		// Management
	case *messages.Error:
		if err := c.handleError(msg); err != nil {
			c.errChan <- err
		}
		c.stateChan <- c.state
	case *messages.Notify:
		if err := c.handleNotify(msg); err != nil {
			fmt.Println("err on Notify")
			c.errChan <- err
		}
		c.stateChan <- c.state
		// Others: SSNM and RKM is not implemented.
	case *messages.RegReq:
		fmt.Println("req")

		p := &params.Param{}
		p.Tag = params.RegistrationResult
		p.Length = 28
		p.Data = []byte{0x2, 0x8, 0x0, 0x1c, 0x2, 0xa, 0x0, 0x8, 0x0, 0x0, 0x0, 0x1, 0x2, 0x12, 0x0, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6, 0x0, 0x8, 0x0, 0x0, 0x0, 0x6}

		if _, err := c.WriteSignal(
			messages.NewRegRsp(p),
		); err != nil {
			c.errChan <- err
			return
		}

		s := &params.Param{}
		s.Tag = params.RegistrationResult
		s.Length = 8
		s.Data = []byte{0x0, 0xd, 0x0, 0x8, 0x0, 0x1, 0x0, 0x2}

		x := &params.Param{}
		x.Tag = params.RegistrationResult
		x.Length = 8
		x.Data = []byte{0x0, 0x6, 0x0, 0x8, 0x0, 0x0, 0x0, 0x6}

		/*
			if _, err := c.WriteSignal(
				messages.NewNotify(s, nil, x, nil),
			); err != nil {
				c.errChan <- err
				return
			}
		*/

		c.stateChan <- c.state

	case *messages.RegRsp:
		fmt.Println("resp")
		c.stateChan <- c.state

		// todo: damn, re-write m3ua from scratch!
		b, _ := m3.MarshalBinary()
		rr, err := messages.ParseRegRsp(b)
		if err != nil {
			c.errChan <- err
			return
		}
		rc, err := rr.FetchRC()
		if err != nil {
			c.errChan <- err
			return
		}
		c.cfg.RoutingContexts = params.NewRoutingContext(rc)
	default:
		fmt.Println("UNHANDLED:", c.state, m3.MessageClassName(), m3.MessageTypeName(), m3.MessageClass(), m3.MessageType())

		c.errChan <- NewErrUnsupportedMessage(m3)
		c.stateChan <- c.state
	}
}

func (c *Conn) monitor(ctx context.Context) {
	fmt.Println("new monitor", c.mode, c.id)
	defer fmt.Println("monitor done", c.mode, c.id)
	c.errChan = make(chan error)
	c.beatAckChan = make(chan struct{})

	c.beatAllow = sync.NewCond(&sync.Mutex{})
	c.beatAllow.L.Lock()
	go c.heartbeat(ctx)
	defer c.beatAllow.Broadcast()

	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			fmt.Println("monitor, ctx done")
			c.Close()
			return
		case err := <-c.errChan:
			fmt.Println("errChan", err)
			if e := c.handleErrors(err); e != nil {
				c.Close()
				return
			}
			// continue
			fmt.Println("closing anyway")
			c.Close()
			return
		case state := <-c.stateChan:
			// Act properly based on current state.
			if err := c.handleStateUpdate(state); err != nil {
				if errors.Is(err, ErrSCTPNotAlive) {
					fmt.Println("monitor, handleStateUpdate", err)
					c.Close()
					return
				}
			}

			// Read from conn to see something coming from the peer.
			n, _, err := c.sctpConn.SCTPRead(buf)
			if err != nil {
				fmt.Println("monitor, sctp read", err)
				c.Close()
				return
			}

			raw := make([]byte, n)
			copy(raw, buf)
			go func() {
				// Parse the received packet as M3UA. Undecodable packets are ignored.
				msg, err := messages.Parse(raw)
				if err != nil {
					logf("failed to parse M3UA message: %v, %x", err, raw)
					return
				}

				c.handleSignals(ctx, msg)
			}()
		}
	}
}

// Copyright 2018-2024 go-m3ua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package m3ua

import (
	"fmt"

	"github.com/dmisol/go-m3ua/messages"
)

func (c *Conn) initiateASPSM() error {
	if _, err := c.WriteSignal(
		messages.NewAspUp(c.cfg.AspIdentifier, nil),
	); err != nil {
		return err
	}

	return nil
}
func (c *Conn) handleAspUp(aspUp *messages.AspUp) error {
	if c.state != StateAspDown {
		// return NewErrUnexpectedMessage(aspUp)
		fmt.Println("handleAspUp, was error here:", NewErrUnexpectedMessage(aspUp))
	}
	if c.sctpInfo.Stream != 0 {
		fmt.Println("handleAspUp, c.sctpInfo.Stream != 0")
		return NewErrInvalidSCTPStreamID(c.sctpInfo.Stream)
	}

	if _, err := c.WriteSignal(
		messages.NewAspUpAck(
			c.cfg.AspIdentifier,
			nil,
		),
	); err != nil {
		fmt.Println("handleAspUp, WriteSignal(NewAspUpAck)", err)
		return err
	}

	fmt.Println("AspUpAck sent")
	return nil
}

func (c *Conn) handleAspUpAck(aspUpAck *messages.AspUpAck) error {
	if c.state != StateAspDown {
		return NewErrUnexpectedMessage(aspUpAck)
	}
	if c.sctpInfo.Stream != 0 {
		return NewErrInvalidSCTPStreamID(c.sctpInfo.Stream)
	}

	return nil
}

func (c *Conn) handleAspDown(aspDown *messages.AspDown) error {
	switch c.state {
	case StateAspInactive, StateAspActive:
		return NewErrUnexpectedMessage(aspDown)
	}
	if c.sctpInfo.Stream != 0 {
		return NewErrInvalidSCTPStreamID(c.sctpInfo.Stream)
	}

	// XXX - Validate the params.

	if _, err := c.WriteSignal(messages.NewAspDownAck(nil)); err != nil {
		return err
	}

	return nil
}

func (c *Conn) handleAspDownAck(aspDownAck *messages.AspDownAck) error {
	switch c.state {
	case StateAspInactive, StateAspActive:
		return NewErrUnexpectedMessage(aspDownAck)
	}
	if c.sctpInfo.Stream != 0 {
		return NewErrInvalidSCTPStreamID(c.sctpInfo.Stream)
	}

	return nil
}

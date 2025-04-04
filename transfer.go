// Copyright 2018-2024 go-m3ua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package m3ua

import (
	"context"
	"errors"

	"github.com/dmisol/go-m3ua/messages"
)

func (c *Conn) handleData(ctx context.Context, data *messages.Data) {
	err := func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.state != StateAspActive {
			c.errChan <- NewErrUnexpectedMessage(data)
			return errors.New(data.String())
		}
		return nil
	}()
	if err != nil {
		// it has already emitted the error into the errChan, early exit
		return
	}

	pd, err := data.ProtocolData.ProtocolData()
	if err != nil {
		c.errChan <- ErrFailedToPeelOff
		return
	}
	// fmt.Println("data, serve event")
	e := &ServeEvent{
		PD: pd,
		Id: c.id,
	}

	if c.cfg.SelfSPC != pd.DestinationPointCode {
		c.errChan <- NewErrUnexpectedMessage(data)
		return
	}

	if c.cfg.DefaultDPC == 0 {
		c.cfg.DefaultDPC = pd.OriginatingPointCode
	}

	// fmt.Println("preparing to send event")

	select {
	case c.serviceChan <- e:
		// fmt.Println("event sent")
		return
	case <-ctx.Done():
		return
	}
}

// Copyright 2019 shimingyah. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// ee the License for the specific language governing permissions and
// limitations under the License.

package pool

import (
	"context"
	"google.golang.org/grpc"
	"time"
)

// Conn single grpc connection inerface
type Conn interface {
	// Value return the actual grpc connection type *grpc.ClientConn.
	Value() *grpc.ClientConn

	// Close decrease the reference of grpc connection, instead of close it.
	// if the pool is full, just close it.
	Close() error

	Context() context.Context

	NewContext(timeout ...time.Duration)
}

// Conn is wrapped grpc.ClientConn. to provide close and value method.
type conn struct {
	cc            *grpc.ClientConn
	pool          *pool
	once          bool
	context       context.Context
	contextCancel context.CancelFunc
}

// Value see Conn interface.
func (c *conn) Value() *grpc.ClientConn {
	return c.cc
}

// Close see Conn interface.
func (c *conn) Close() error {
	c.pool.decrRef()
	if c.contextCancel != nil {
		c.contextCancel()
		c.context = nil
		c.contextCancel = nil
	}
	if c.once {
		return c.reset()
	}
	return nil
}

// Context see Conn interface.
func (c *conn) Context() context.Context {
	return c.context
}

// NewContext see Conn interface.
func (c *conn) NewContext(timeout ...time.Duration) {
	if len(timeout) == 0 {
		c.context = context.Background()
		c.contextCancel = nil
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout[0])
	c.context = ctx
	c.contextCancel = cancel
}

func (c *conn) reset() error {
	cc := c.cc
	c.cc = nil
	c.once = false
	if cc != nil {
		return cc.Close()
	}
	return nil
}

func (p *pool) wrapConn(cc *grpc.ClientConn, once bool) *conn {
	return &conn{
		cc:   cc,
		pool: p,
		once: once,
	}
}

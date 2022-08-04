package consul

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"github.com/godaddy-x/freego/component/log"
	"io"
	"net/rpc"
	"time"
)

type gobClientCodec struct {
	rwc     io.ReadWriteCloser
	dec     *gob.Decoder
	enc     *gob.Encoder
	encBuf  *bufio.Writer
	timeout int64
}

func (c *gobClientCodec) WriteRequest(r *rpc.Request, body interface{}) (err error) {
	if err = TimeoutCoder(c.enc.Encode, r, c.timeout, "client write request"); err != nil {
		return
	}
	if err = TimeoutCoder(c.enc.Encode, body, c.timeout, "client write request body"); err != nil {
		return
	}
	return c.encBuf.Flush()
}

func (c *gobClientCodec) ReadResponseHeader(r *rpc.Response) error {
	return c.dec.Decode(r)
}

func (c *gobClientCodec) ReadResponseBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *gobClientCodec) Close() error {
	return c.rwc.Close()
}

func TimeoutCoder(f func(interface{}) error, e interface{}, timeout int64, msg string) error {
	echan := make(chan error, 1)
	go func() {
		echan <- f(e)
	}()
	select {
	case e := <-echan:
		return e
	case <-time.After(time.Second*time.Duration(timeout)): // connect timeout - 5s
		return fmt.Errorf("TimeoutCoder failed: %s", msg)
	}
}

type gobServerCodec struct {
	rwc     io.ReadWriteCloser
	dec     *gob.Decoder
	enc     *gob.Encoder
	encBuf  *bufio.Writer
	closed  bool
	timeout int64
}

func (c *gobServerCodec) ReadRequestHeader(r *rpc.Request) error {
	return TimeoutCoder(c.dec.Decode, r, c.timeout, "server read request header")
}

func (c *gobServerCodec) ReadRequestBody(body interface{}) error {
	return TimeoutCoder(c.dec.Decode, body, c.timeout, "server read request body")
}

func (c *gobServerCodec) WriteResponse(r *rpc.Response, body interface{}) (err error) {
	if err = TimeoutCoder(c.enc.Encode, r, c.timeout, "server write response"); err != nil {
		if c.encBuf.Flush() == nil {
			log.Error("rpc: gob error encoding response", 0, log.AddError(err))
			c.Close()
		}
		return
	}
	if err = TimeoutCoder(c.enc.Encode, body, c.timeout, "server write response body"); err != nil {
		if c.encBuf.Flush() == nil {
			log.Error("rpc: gob error encoding body", 0, log.AddError(err))
			c.Close()
		}
		return
	}
	return c.encBuf.Flush()
}

func (c *gobServerCodec) Close() error {
	if c.closed {
		// Only call c.rwc.Close once; otherwise the semantics are undefined.
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}

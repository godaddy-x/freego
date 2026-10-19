package grpcgw

import (
	"bytes"
	"testing"
)

func TestSetAuthTokenCachesSubject(t *testing.T) {
	c := &Client{}
	c.SetAuthToken("jwt", bytes.Repeat([]byte{1}, 32), 0, "42")
	if c.Subject() != "42" {
		t.Fatalf("subject=%q want 42", c.Subject())
	}
	sess := c.CopySession()
	if sess.Subject != "42" {
		t.Fatalf("session subject=%q want 42", sess.Subject)
	}
}

func TestSetAuthTokenCachesStringSubject(t *testing.T) {
	c := &Client{}
	c.SetAuthToken("jwt", bytes.Repeat([]byte{1}, 32), 0, "platform-node")
	if c.Subject() != "platform-node" {
		t.Fatalf("subject=%q want platform-node", c.Subject())
	}
}

package grpcgw

import (
	"bytes"
	"sync"
	"testing"
)

func TestCopySessionSurvivesTokenReplace(t *testing.T) {
	c := &Client{clientNo: 7}
	oldSecret := bytes.Repeat([]byte{0x11}, 32)
	c.SetAuthToken("tok-old", oldSecret, 100, "1")
	sess := c.CopySession()

	c.SetAuthToken("tok-new", bytes.Repeat([]byte{0x22}, 32), 200, "1")

	if sess.Token != "tok-old" || sess.Expired != 100 || sess.ClientNo != 7 {
		t.Fatalf("snapshot changed: %+v", sess)
	}
	if sess.Secret[0] != 0x11 {
		t.Fatal("snapshot secret was overwritten")
	}
	if c.Token() != "tok-new" || c.Expired() != 200 {
		t.Fatal("client did not switch to new token")
	}
	mac, err := sess.MACKey()
	if err != nil || mac[0] != 0x11 {
		t.Fatalf("macKey=%v err=%v", mac, err)
	}
}

func TestCopySessionConcurrentReplace(t *testing.T) {
	c := &Client{clientNo: 1}
	c.SetAuthToken("tok", bytes.Repeat([]byte{1}, 32), 10, "1")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for n := 0; n < 50; n++ {
				sess := c.CopySession()
				if sess.Token == "" || len(sess.Secret) < 32 {
					t.Errorf("torn session token=%q secret=%d", sess.Token, len(sess.Secret))
					return
				}
			}
		}()
		go func(id int) {
			defer wg.Done()
			sec := bytes.Repeat([]byte{byte(id + 1)}, 32)
			for n := 0; n < 50; n++ {
				c.SetAuthToken("tok", sec, int64(100+n), "1")
			}
		}(i)
	}
	wg.Wait()
}

package concurrent

import (
	"sync"
	"sync/atomic"
)

type Once struct {
	done uint32
	m    sync.Mutex
}

func (o *Once) Do(f func() (interface{}, error)) (interface{}, error) {
	if atomic.LoadUint32(&o.done) == 0 {
		return o.doSlow(f)
	}
	return nil, nil
}

func (o *Once) doSlow(f func() (interface{}, error)) (interface{}, error) {
	o.m.Lock()
	defer o.m.Unlock()
	if o.done == 0 {
		defer atomic.StoreUint32(&o.done, 1)
		return f()
	}
	return nil, nil
}

package socks5

import (
	"context"
	"net"
	"sync"
)

type trackingListener struct {
	net.Listener
	mu      sync.Mutex
	conns   map[*trackedConn]struct{}
	wg      sync.WaitGroup
	closing bool
}

func newTrackingListener(inner net.Listener) *trackingListener {
	return &trackingListener{
		Listener: inner,
		conns:    make(map[*trackedConn]struct{}),
	}
}

func (l *trackingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	tracked := &trackedConn{Conn: conn, listener: l}
	l.conns[tracked] = struct{}{}
	l.wg.Add(1)
	l.mu.Unlock()
	return tracked, nil
}

func (l *trackingListener) Close() error {
	l.mu.Lock()
	l.closing = true
	l.mu.Unlock()
	return l.Listener.Close()
}

// wait blocks until every accepted connection closes or ctx expires. On
// timeout the remaining connections are force-closed so Stop cannot hang.
func (l *trackingListener) wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-ctx.Done():
	}
	l.closeAll()
	<-done
}

func (l *trackingListener) closeAll() {
	l.mu.Lock()
	conns := make([]*trackedConn, 0, len(l.conns))
	for conn := range l.conns {
		conns = append(conns, conn)
	}
	l.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

type trackedConn struct {
	net.Conn
	listener *trackingListener
	once     sync.Once
}

func (c *trackedConn) Close() error {
	c.once.Do(func() {
		c.listener.mu.Lock()
		delete(c.listener.conns, c)
		c.listener.mu.Unlock()
		c.listener.wg.Done()
	})
	return c.Conn.Close()
}

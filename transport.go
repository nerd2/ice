package ice

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/pion/stun"
)

// Dial connects to the remote agent, acting as the controlling ice agent.
// Dial blocks until at least one ice candidate pair has successfully connected.
func (a *Agent) Dial(ctx context.Context, remoteUfrag, remotePwd string) (*Conn, error) {
	return a.connect(ctx, true, remoteUfrag, remotePwd)
}

// Accept connects to the remote agent, acting as the controlled ice agent.
// Accept blocks until at least one ice candidate pair has successfully connected.
func (a *Agent) Accept(ctx context.Context, remoteUfrag, remotePwd string) (*Conn, error) {
	return a.connect(ctx, false, remoteUfrag, remotePwd)
}

// Conn represents the ICE connection.
// At the moment the lifetime of the Conn is equal to the Agent.
type Conn struct {
	bytesReceived uint64
	bytesSent     uint64
	agent         *Agent
}

// BytesSent returns the number of bytes sent
func (c *Conn) BytesSent() uint64 {
	return atomic.LoadUint64(&c.bytesSent)
}

// BytesReceived returns the number of bytes received
func (c *Conn) BytesReceived() uint64 {
	return atomic.LoadUint64(&c.bytesReceived)
}

func (a *Agent) connect(ctx context.Context, isControlling bool, remoteUfrag, remotePwd string) (*Conn, error) {
	err := a.ok()
	if err != nil {
		return nil, err
	}
	err = a.startConnectivityChecks(isControlling, remoteUfrag, remotePwd)
	if err != nil {
		return nil, err
	}

	// block until pair selected
	select {
	case <-a.done:
		return nil, a.getErr()
	case <-ctx.Done():
		// TODO: Stop connectivity checks?
		return nil, ErrCanceledByCaller
	case <-a.onConnected:
	}

	return &Conn{
		agent: a,
	}, nil
}

// Read implements the Conn Read method.
func (c *Conn) Read(p []byte) (int, error) {
	err := c.agent.ok()
	if err != nil {
		return 0, err
	}

	n, err := c.agent.buffer.Read(p)
	atomic.AddUint64(&c.bytesReceived, uint64(n))
	return n, err
}

// Write implements the Conn Write method.
func (c *Conn) Write(p []byte) (int, error) {
	err := c.agent.ok()
	if err != nil {
		return 0, err
	}

	if stun.IsMessage(p) {
		return 0, errors.New("the ICE conn can't write STUN messages")
	}

	pair := c.agent.getSelectedPair()
	if pair == nil {
		bestValidPair := make(chan *candidatePair, 1)
		if err = c.agent.run(func(a *Agent) {
			bestValidPair <- a.getBestValidCandidatePair()
		}, nil); err != nil {
			return 0, err
		}

		pair = <-bestValidPair
		if pair == nil {
			return 0, err
		}
	}

	atomic.AddUint64(&c.bytesSent, uint64(len(p)))
	return pair.Write(p)
}

// Close implements the Conn Close method. It is used to close
// the connection. Any calls to Read and Write will be unblocked and return an error.
func (c *Conn) Close() error {
	return c.agent.Close()
}

// TODO: Maybe just switch to using io.ReadWriteCloser?

// LocalAddr is a stub
func (c *Conn) LocalAddr() net.Addr {
	return nil
}

// RemoteAddr is a stub
func (c *Conn) RemoteAddr() net.Addr {
	return nil
}

// SetDeadline is a stub
func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline is a stub
func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline is a stub
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

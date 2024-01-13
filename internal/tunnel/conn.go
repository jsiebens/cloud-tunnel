package tunnel

import (
	"io"
	"net"
	"time"
)

type rwcConn struct {
	addr string
	rwc  io.ReadWriteCloser
}

func (conn rwcConn) Read(p []byte) (int, error)         { return conn.rwc.Read(p) }
func (conn rwcConn) Write(p []byte) (int, error)        { return conn.rwc.Write(p) }
func (conn rwcConn) Close() error                       { return conn.rwc.Close() }
func (conn rwcConn) LocalAddr() net.Addr                { return rwcAddr{conn.addr} }
func (conn rwcConn) RemoteAddr() net.Addr               { return nil }
func (conn rwcConn) SetDeadline(t time.Time) error      { return nil }
func (conn rwcConn) SetReadDeadline(t time.Time) error  { return nil }
func (conn rwcConn) SetWriteDeadline(t time.Time) error { return nil }

type rwcAddr struct {
	v string
}

func (addr rwcAddr) Network() string { return "tcp" }
func (addr rwcAddr) String() string  { return addr.v }

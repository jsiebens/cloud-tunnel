package iap

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"io"
	"net"
	"net/http"
	"net/url"
	"nhooyr.io/websocket"
)

var _ net.Conn = (*Conn)(nil)

const (
	proxySubproto = "relay.tunnel.cloudproxy.app"
	proxyHost     = "tunnel.cloudproxy.app"
	proxyPath     = "/v4/connect"
	proxyOrigin   = "bot:iap-tunneler"
)

const (
	subprotoMaxFrameSize        = 16384
	subprotoTagSuccess   uint16 = 0x1
	subprotoTagData      uint16 = 0x4
	subprotoTagAck       uint16 = 0x7
)

// Dial connects to the IAP proxy and returns a Conn or error if the connection fails.
func Dial(ctx context.Context, ts oauth2.TokenSource, opts DialOptions) (*Conn, error) {
	header := make(http.Header)
	header.Set("Origin", proxyOrigin)

	if ts != nil {
		token, err := ts.Token()
		if err != nil {
			return nil, err
		}

		header.Set("Authorization", fmt.Sprintf("%v %v", token.Type(), token.AccessToken))
	}

	wsOptions := websocket.DialOptions{
		HTTPHeader:      header,
		Subprotocols:    []string{proxySubproto},
		CompressionMode: websocket.CompressionDisabled,
	}

	conn, _, err := websocket.Dial(ctx, opts.connectURL().String(), &wsOptions)
	if err != nil {
		return nil, err
	}

	netConn := websocket.NetConn(context.Background(), conn, websocket.MessageBinary)

	recvReader, recvWriter := io.Pipe()

	c := &Conn{
		Conn:       netConn,
		recvBuf:    make([]byte, subprotoMaxFrameSize),
		recvReader: recvReader,
		recvWriter: recvWriter,
		sendBuf:    make([]byte, subprotoMaxFrameSize),
	}

	if err := c.readFrame(); err != nil {
		var closeError websocket.CloseError
		if errors.As(err, &closeError) {
			return nil, fmt.Errorf("connection closed: code %v (%v)", int(closeError.Code), closeError.Reason)
		}

		return nil, err
	}

	go c.read()

	return c, nil
}

type DialOptions struct {
	Project  string
	Zone     string
	Instance string
	Port     int
}

func (d DialOptions) connectURL() *url.URL {
	query := url.Values{
		"zone":      []string{d.Zone},
		"project":   []string{d.Project},
		"port":      []string{fmt.Sprintf("%d", d.Port)},
		"interface": []string{"nic0"},
		"instance":  []string{d.Instance},
	}

	for key, value := range query {
		if value[0] == "" {
			query.Del(key)
		}
	}

	return &url.URL{
		Scheme:   "wss",
		Host:     proxyHost,
		Path:     proxyPath,
		RawQuery: query.Encode(),
	}
}

type Conn struct {
	net.Conn
	connected bool

	recvNbAcked   uint64
	recvNbUnacked uint64
	recvBuf       []byte
	recvReader    *io.PipeReader
	recvWriter    *io.PipeWriter

	sendNbAcked   uint64
	sendNbUnacked uint64
	sendBuf       []byte
}

func (c *Conn) Close() error {
	_ = c.recvReader.Close()
	_ = c.recvWriter.Close()
	return c.Conn.Close()
}

func (c *Conn) Read(buf []byte) (n int, err error) {
	return c.recvReader.Read(buf)
}

func (c *Conn) Write(data []byte) (n int, err error) {
	total := len(data)
	reader := bytes.NewReader(data)

	nb := total
	for nb > 0 {
		// clamp each write to max frame size
		writeNb := min(nb, subprotoMaxFrameSize)
		nb -= writeNb

		var buf bytes.Buffer

		_ = binary.Write(&buf, binary.BigEndian, subprotoTagData)
		_ = binary.Write(&buf, binary.BigEndian, uint32(writeNb))

		if _, err := copyNBuffer(&buf, reader, int64(writeNb), c.sendBuf); err != nil {
			return 0, err
		}

		writtenNb, err := c.Conn.Write(buf.Bytes())
		if err != nil {
			return 0, err
		}

		c.sendNbUnacked += uint64(writtenNb)
	}

	return total, nil
}

func (c *Conn) writeAck(nb uint64) error {
	buf := make([]byte, 10)

	binary.BigEndian.PutUint16(buf[0:2], subprotoTagAck)
	binary.BigEndian.PutUint64(buf[2:10], nb)

	_, err := c.Conn.Write(buf)
	return err
}

func (c *Conn) readSuccessFrame(r io.Reader) error {
	buf := [4]byte{}
	if _, err := r.Read(buf[:]); err != nil {
		return err
	}
	frameLen := binary.BigEndian.Uint32(buf[:])

	if frameLen > subprotoMaxFrameSize {
		return fmt.Errorf("len exceeds subprotocol max data frame size")
	}

	sessionID := make([]byte, frameLen)
	if _, err := r.Read(sessionID); err != nil {
		return err
	}

	c.connected = true
	return nil
}

func (c *Conn) readAckFrame(r io.Reader) error {
	buf := [8]byte{}
	if _, err := r.Read(buf[:]); err != nil {
		return err
	}

	c.sendNbAcked = binary.BigEndian.Uint64(buf[:])
	return nil
}

func (c *Conn) readDataFrame(r io.Reader) error {
	buf := [4]byte{}
	if _, err := r.Read(buf[:]); err != nil {
		return err
	}
	frameLen := binary.BigEndian.Uint32(buf[:])

	if frameLen > subprotoMaxFrameSize {
		return fmt.Errorf("len exceeds subprotocol max data frame size")
	}

	if _, err := copyNBuffer(c.recvWriter, r, int64(frameLen), c.recvBuf); err != nil {
		return err
	}

	c.recvNbUnacked += uint64(frameLen)
	return nil
}

func (c *Conn) readFrame() error {
	buf := [2]byte{}
	if _, err := c.Conn.Read(buf[:]); err != nil {
		return err
	}
	tag := binary.BigEndian.Uint16(buf[:])

	var err error

	switch tag {
	case subprotoTagSuccess:
		err = c.readSuccessFrame(c.Conn)
	default:
		if !c.connected {
			return fmt.Errorf("received frame before connection was established")
		}

		switch tag {
		case subprotoTagAck:
			err = c.readAckFrame(c.Conn)
		case subprotoTagData:
			err = c.readDataFrame(c.Conn)

			if c.recvNbUnacked-c.recvNbAcked > 2*subprotoMaxFrameSize {
				if err := c.writeAck(c.recvNbUnacked); err != nil {
					return err
				}
				c.recvNbAcked = c.recvNbUnacked
			}
		default:
			// unknown tags should be ignored
			return nil
		}

	}

	return err
}

func (c *Conn) read() {
	for {
		if err := c.readFrame(); err != nil {
			_ = c.Close()
			return
		}
	}
}

// copyNBuffer is like io.CopyN but stages through a given buffer like io.CopyBuffer.
func copyNBuffer(w io.Writer, r io.Reader, n int64, buf []byte) (int64, error) {
	return io.CopyBuffer(w, io.LimitReader(r, n), buf)
}

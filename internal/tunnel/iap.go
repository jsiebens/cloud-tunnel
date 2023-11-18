package tunnel

import (
	"context"
	"fmt"
	"github.com/cedws/iapc/iap"
	"golang.org/x/oauth2/google"
	"net"
	"sync/atomic"
)

func dial(ctx context.Context, instance string, port int, project, zone string) (net.Conn, error) {
	ts, err := google.DefaultTokenSource(ctx)
	if err != nil {
		return nil, err
	}

	opts := []iap.DialOption{
		iap.WithProject(project),
		iap.WithInstance(instance, zone, "nic0"),
		iap.WithPort(fmt.Sprintf("%d", port)),
		iap.WithTokenSource(&ts),
	}

	conn, err := iap.Dial(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return &closeOnceConn{Conn: conn}, nil
}

type closeOnceConn struct {
	net.Conn
	closed uint32
}

func (c *closeOnceConn) Close() error {
	if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		return c.Conn.Close()
	}
	return nil
}

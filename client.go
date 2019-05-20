package stun

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/transport/vnet"
	"github.com/pkg/errors"
)

var (
	maxMessageSize = 1280

	// ErrResponseTooBig is returned if more than maxMessageSize bytes are returned in the response
	// see https://tools.ietf.org/html/rfc5389#section-7 for the size limit
	ErrResponseTooBig = errors.New("received too much data")
)

// ClientConfig is a set of configuration parameters for NewClient().
type ClientConfig struct {
	Protocol string
	Server   string
	Deadline time.Duration
	Net      *vnet.Net
}

// Client is a STUN client that sents STUN requests and receives STUN responses
type Client struct {
	conn net.Conn
	net  *vnet.Net
}

// NewClient creates a configured STUN client
func NewClient(config *ClientConfig) (*Client, error) {
	nw := config.Net
	if nw == nil {
		nw = vnet.NewNet(nil) // defaults to normal operation
	}
	dialer := nw.CreateDialer(&net.Dialer{
		Timeout: config.Deadline,
	})
	conn, err := dialer.Dial(config.Protocol, config.Server)
	if err != nil {
		return nil, err
	}
	err = conn.SetReadDeadline(time.Now().Add(config.Deadline))
	if err != nil {
		return nil, err
	}
	err = conn.SetWriteDeadline(time.Now().Add(config.Deadline))
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn,
		net:  nw,
	}, nil
}

// LocalAddr returns local address of the client
func (c *Client) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// Close disconnects the client
func (c *Client) Close() error {
	return c.conn.Close()
}

// Request executes a STUN request against the clients server
func (c *Client) Request() (*Message, error) {
	return request(c.conn.Read, c.conn.Write)
}

// GetMappedAddressUDP initiates a stun requests to serverAddr using conn, reads the response and returns
// the XorAddress returned by the stun server via the AttrXORMappedAddress attribute
func GetMappedAddressUDP(conn net.PacketConn, serverAddr net.Addr, deadline time.Duration) (*XorAddress, error) {
	var err error

	udpConn, ok := conn.(vnet.UDPPacketConn)
	if !ok {
		return nil, fmt.Errorf("not vnet.UDPConn")
	}

	if deadline > 0 {
		err = udpConn.SetReadDeadline(time.Now().Add(deadline))
		if err != nil {
			return nil, err
		}
		err = udpConn.SetWriteDeadline(time.Now().Add(deadline))
		if err != nil {
			return nil, err
		}
	}

	resp, err := request(
		udpConn.Read,
		func(b []byte) (int, error) {
			return udpConn.WriteTo(b, serverAddr)
		},
	)
	if err != nil {
		return nil, err
	}

	if deadline > 0 {
		err = udpConn.SetReadDeadline(time.Time{})
		if err != nil {
			return nil, err
		}
		err = udpConn.SetWriteDeadline(time.Time{})
		if err != nil {
			return nil, err
		}
	}

	attr, ok := resp.GetOneAttribute(AttrXORMappedAddress)
	if !ok {
		return nil, fmt.Errorf("got response from STUN server that did not contain XORAddress")
	}

	addr := &XorAddress{}
	if err = addr.Unpack(resp, attr); err != nil {
		return nil, fmt.Errorf("failed to unpack STUN XorAddress response: %v", err)
	}

	return addr, nil
}

func request(read func([]byte) (int, error), write func([]byte) (int, error)) (*Message, error) {
	req, err := Build(ClassRequest, MethodBinding, GenerateTransactionID())
	if err != nil {
		return nil, err
	}

	_, err = write(req.Pack())
	if err != nil {
		return nil, err
	}

	bs := make([]byte, maxMessageSize)
	n, err := read(bs)
	if err != nil {
		return nil, err
	}
	if n > maxMessageSize {
		return nil, ErrResponseTooBig
	}

	return NewMessage(bs[:n])
}

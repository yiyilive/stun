package stun

import (
	"fmt"
	"net"
	"sync"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
)

const (
	maxReceiveBufferSize = 2 * 1024
)

// ServerConfig is config parameters for STUN server.
type ServerConfig struct {
	LoggerFactory logging.LoggerFactory
	Net           *vnet.Net
}

// Server is implements STUN server.
type Server struct {
	loggerFactory logging.LoggerFactory
	log           logging.LeveledLogger
	net           *vnet.Net
	udpListeners  map[string]net.PacketConn
	mutex         sync.RWMutex
}

// NewServer creates a new instance of STUN server.
func NewServer(config *ServerConfig) *Server {
	nw := config.Net
	if nw == nil {
		nw = vnet.NewNet(nil)
	}

	return &Server{
		loggerFactory: config.LoggerFactory,
		log:           config.LoggerFactory.NewLogger("stun"),
		net:           nw,
		udpListeners:  map[string]net.PacketConn{},
	}
}

// AddListener adds a new listener.
func (s *Server) AddListener(network, address string) (net.Addr, error) {
	s.log.Debug("AddListener() called")

	switch network {
	case "udp":
	case "udp4":
	case "udp6":
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	s.log.Debug("AddListener() - pt1")
	udpConn, err := s.net.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}

	lAddr, ok := udpConn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("failed to switch type")
	}

	s.log.Debugf("lAddr: %v", lAddr)

	s.log.Debug("AddListener() - pt2")

	tpAddr := fmt.Sprintf("%s:%s",
		lAddr.Network(),
		lAddr.String(),
	)
	s.udpListeners[tpAddr] = udpConn

	s.log.Infof("start listening on %s", tpAddr)

	go func() {
		buf := make([]byte, maxReceiveBufferSize)
		for {
			n, addr, err := udpConn.ReadFrom(buf)
			if err != nil {
				s.log.Warnf("stop listening on %s", tpAddr)
				break
			}

			resp, err := s.onPacketReceived(udpConn, buf[:n], addr)
			if err != nil {
				s.log.Errorf("internal error: %v", err)
				break
			}

			if resp != nil {
				_, err = udpConn.WriteTo(resp, addr)
				if err != nil {
					s.log.Errorf("write failed: %v", err)
					break
				}
			}
		}

		delete(s.udpListeners, tpAddr)
		s.log.Infof("listener %s exited", tpAddr)
	}()

	return lAddr, nil
}

// ShutDown shuts down the server.
func (s *Server) ShutDown() {
	for tpAddr, conn := range s.udpListeners {
		err := conn.Close()
		if err != nil {
			s.log.Errorf("failed to close listener %s, Error: %v", tpAddr, err)
		}
	}
}

func (s *Server) onPacketReceived(conn net.PacketConn, data []byte, from net.Addr) ([]byte, error) {
	msg, err := NewMessage(data)
	if err != nil {
		return nil, err
	}

	if msg.Class != ClassRequest {
		s.log.Warnf("received unexpected message class: %d", msg.Class)
		return nil, nil
	}

	if msg.Method != MethodBinding {
		s.log.Warnf("received unexpected message method class: %d", msg.Method)
		return nil, nil
	}

	if from == nil {
		return nil, fmt.Errorf("from addr cannot be nil")
	}

	fromAddr, ok := from.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("failed to type-switch to *net.UDPAddr")
	}

	resp, err := Build(
		ClassSuccessResponse,
		MethodBinding,
		msg.TransactionID,
		&XorMappedAddress{
			XorAddress: XorAddress{
				IP:   fromAddr.IP,
				Port: fromAddr.Port,
			},
		},
	)
	if err != nil {
		return nil, err
	}

	return resp.Pack(), nil

}

package stun

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("Normal", func(t *testing.T) {
		// Run server
		s := NewServer(&ServerConfig{
			LoggerFactory: loggerFactory,
		})
		svrAddr, err := s.AddListener("udp4", "127.0.0.1:3456")
		assert.NoError(t, err, "should succeed")
		log.Debugf("listening on %s", svrAddr.(*net.UDPAddr).String())
		assert.Equal(t, "127.0.0.1:3456", svrAddr.String(), "should succeed")

		// Perform GetMappedAddressUDP()
		conn, err := net.ListenPacket("udp4", "127.0.0.1:5678")
		if err != nil {
			t.Fatal(err)
		}
		cliAddr := conn.LocalAddr().(*net.UDPAddr)

		xoraddr, err := GetMappedAddressUDP(
			conn,
			svrAddr,
			time.Second*1,
		)
		assert.NoError(t, err, "should succeed")
		assert.True(t, cliAddr.IP.Equal(xoraddr.IP), "should match")
		assert.Equal(t, cliAddr.Port, xoraddr.Port, "should match")

		s.ShutDown()
	})

	t.Run("Use vnet", func(t *testing.T) {
		// Create a virtual network
		wan, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR:          "0.0.0.0/0",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err, "should succeed")

		lan, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR:          "192.168.1.0/24",
			StaticIP:      "5.6.7.8",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err, "should succeed")

		err = wan.AddRouter(lan)
		assert.NoError(t, err, "should succeed")

		sNet := vnet.NewNet(&vnet.NetConfig{
			StaticIP: "1.2.3.4",
		})
		err = wan.AddNet(sNet)
		assert.NoError(t, err, "should succeed")

		cNet := vnet.NewNet(&vnet.NetConfig{})
		err = lan.AddNet(cNet)
		assert.NoError(t, err, "should succeed")

		// Run server
		s := NewServer(&ServerConfig{
			LoggerFactory: loggerFactory,
			Net:           sNet,
		})

		svrAddr, err := s.AddListener("udp4", "1.2.3.4:3456")
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		log.Debugf("listening on %s", svrAddr.(*net.UDPAddr).String())
		assert.Equal(t, "1.2.3.4:3456", svrAddr.String(), "should succeed")

		err = wan.Start()
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		// Perform GetMappedAddressUDP()
		conn, err := cNet.ListenPacket("udp4", "192.168.1.1:0")
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		cliAddr, ok := conn.LocalAddr().(*net.UDPAddr)
		if !assert.True(t, ok, "should succeed") {
			return
		}
		log.Debugf("cliAddr: %s", cliAddr.String())

		xoraddr, err := GetMappedAddressUDP(
			conn,
			svrAddr,
			time.Second*1,
		)

		log.Debugf("xoraddr.IP: %v", xoraddr.IP.String())
		log.Debugf("cliAddr.IP: %v", cliAddr.IP.String())

		assert.NoError(t, err, "should succeed")
		assert.Equal(t, "5.6.7.8", xoraddr.IP.String(), "should match")

		err = wan.Stop()
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		s.ShutDown()
	})
}

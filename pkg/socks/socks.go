package socks

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

// Each Connection
type SocksConnection struct {
	Connection      net.Conn
	ConnectionId    string
	IsClosed        bool
	BlockingChannel chan int
}

// AllConnection
type SocksConnectionList struct {
	sync.Mutex
	Connections map[string]SocksConnection
}

func NewSocksConnectionsList() *SocksConnectionList {
	return &SocksConnectionList{
		Connections: make(map[string]SocksConnection, 1),
	}
}

type SocksProxy interface {
	Serve() error
	Shutdown(ctx context.Context, cancel context.CancelFunc) error
	HandleClient(c SocksConnection)
	SetMaxConnections(max int)
	SetIdleTimeout(idleSeconds int)
	AddConnection(c SocksConnection) error
	RemoveConnection(id string) error
}

func NewSocksProxy(addr string, port int, version int) SocksProxy {
	switch version {
	case 4:
		return &Serverv4{
			ListeningAddr: addr,
			ListeningPort: port,
			Listener:      nil,
			ShutdownChan:  make(chan bool),
			Error:         nil,
			Conns:         NewSocksConnectionsList(),
		}
	case 5:
		return &Serverv5{
			ListeningAddr: addr,
			ListeningPort: port,
			Listener:      nil,
			ShutdownChan:  make(chan bool),
			Error:         nil,
			Conns:         NewSocksConnectionsList(),
		}
	default:
		return nil
	}
}

// Helpers
func validateClientRequest(data []byte, version int) (bool, error) {
	if version == 4 {
		if data[0] != 0x04 {
			return false, fmt.Errorf("Invalid version provided by client.") // Invalid version
		}
		if data[1] != 0x1 && data[1] != 0x2 {
			return false, fmt.Errorf("Invalid command provided by client.") // invalid command not bind or connect
		}
	}
	// TODO v5
	return true, nil
}

func findMethodNoAuth(data [255]byte) bool {
	fmt.Println(data)
	for i := range data {
		fmt.Println(data[i])
		if data[i] == 0x00 {
			return true
		}
	}
	return false
}

func findMethodGSSAPI(data [255]byte) bool {
	fmt.Println(data)
	for i := range data {
		fmt.Println(data[i])
		if data[i] == 0x01 {
			return true
		}
	}
	return false
}
func findMethodBasicAuth(data [255]byte) bool {
	fmt.Println(data)
	for i := range data {
		fmt.Println(data[i])
		if data[i] == 0x02 {
			return true
		}
	}
	return false
}

func ToBytes(s interface{}) []byte {
	// converts to big endian
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, s)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return buf.Bytes()
}

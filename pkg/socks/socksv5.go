package socks

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"time"
	"unsafe"

	"github.com/latortuga71/medias/pkg/data"
	"github.com/latortuga71/medias/pkg/log"
)

type Serverv5 struct {
	ListeningAddr string
	Logger        string
	ListeningPort int
	Listener      net.Listener
	ShutdownChan  chan bool
	Error         error
	Conns         *SocksConnectionList
	// how long should we wait for data to be sent over the tcp connection.
	IdleTimeout        int
	MaxConnections     int
	CurrentConnections int
}

func (s *Serverv5) shutdownAllConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// shutdown deadline exceeded.
			return
		default:
			for k := range s.Conns.Connections {
				s.Conns.Connections[k].BlockingChannel <- 0 // only problem is we are not closing the destination connection but it should close anyways.
			}
			log.Logger.Debug().Msgf("Closed all connections...")
			s.Listener.Close()
			log.Logger.Debug().Msgf("Closed listener...")
			s.ShutdownChan <- true
			return
		}
	}
}

func (s *Serverv5) SetIdleTimeout(idleSeconds int) {
	s.IdleTimeout = idleSeconds
}

func (s *Serverv5) SetMaxConnections(max int) {
	s.MaxConnections = max
}

func (s *Serverv5) AddConnection(c SocksConnection) error {
	s.Conns.Mutex.Lock()
	s.Conns.Connections[c.ConnectionId] = c
	s.CurrentConnections++
	s.Conns.Unlock()
	log.Logger.Debug().Msgf("Added Connection Id %d to connection map", c.ConnectionId)
	return nil
}

func (s *Serverv5) RemoveConnection(id string) error {
	s.Conns.Mutex.Lock()
	if !s.Conns.Connections[id].IsClosed {
		s.Conns.Connections[id].Connection.Close()
	}
	delete(s.Conns.Connections, id)
	s.CurrentConnections--
	s.Conns.Unlock()
	log.Logger.Debug().Msgf("Removed Connection %s from connection map", id)
	log.Logger.Info().Msgf("Connections left %d", s.CurrentConnections)
	return nil
}

func (s *Serverv5) HandleNoAuth(c SocksConnection) {
	// send response saying we are ok with no auth
	resp := data.SocksNegotiateResponse{
		Method:  0x00,
		Version: 0x5,
	}
	wrote, err := c.Connection.Write(ToBytes(resp))
	if err != nil {
		s.RemoveConnection(c.ConnectionId)
		return
	}
	log.Logger.Debug().Msgf("Wrote Method Negotiate Response Bytes %d", wrote)
	// read request details with destination etc
	maxSize := unsafe.Sizeof(data.Socksv5Requestv6{})
	dataBuffer := make([]byte, maxSize)
	readBytes, err := c.Connection.Read(dataBuffer)
	if err != nil {
		if err != io.EOF {
			log.Logger.Error().Msgf("Connection ERROR: %v Closing client connection", err)
			s.RemoveConnection(c.ConnectionId)
			return
		}
	}
	if readBytes <= 0 {
		log.Logger.Error().Msgf("Not Enough Data Read From Socket Expected %d bytes got %d bytes", maxSize, readBytes)
		s.RemoveConnection(c.ConnectionId)
		return
	}
	log.Logger.Debug().Msgf("Read SOCKS5 Request Message: Read %d bytes -> data %s\n", readBytes, hex.EncodeToString(dataBuffer))
	clientRequest := (*data.Socksv5Requestv6)(unsafe.Pointer(&dataBuffer[0]))
	if clientRequest.Version != 0x5 {
		log.Logger.Error().Msgf("Socksv5 Negotiate Request Validation Failed ::: INVALID VERSION PROVIDED %v", err)
		s.RemoveConnection(c.ConnectionId)
		return
	}
	if clientRequest.Cmd == 0x3 {
		log.Logger.Error().Msgf("Socksv5 Negotiate Request Validation Failed ::: INVALID COMMAND PROVIDED %v", err)
		panic("TODO SUPPORT UDP")
	}
	if clientRequest.Cmd == 0x2 {
		log.Logger.Error().Msgf("Socksv5 Negotiate Request Validation Failed ::: INVALID COMMAND PROVIDED %v", err)
		panic("TODO SUPPORT BIND")
	}
	if clientRequest.Cmd == 0x1 {
		log.Logger.Debug().Msgf("Client wants CONNECT COMMAND\n")
		if clientRequest.ATYP != 0x01 {
			panic("TODO SUPPORT IPV6 or domain name")
		}
		// check if we can reach target server on specified port etc.
		destConn, err := s.CheckConnectivityIpv4(dataBuffer)
		if err != nil {
			s.SendResponsev4(c, 0x03, dataBuffer) // network unreachable
			return
		}
		log.Logger.Debug().Msgf("OK! Sending Response And Forwarding data.\n")
		s.SendResponsev4(c, 0x00, dataBuffer) // succeeed
		s.ForwardData(c, destConn)
		// above function blocks.
		return
	}
}

func (s *Serverv5) ForwardData(c SocksConnection, destinationConn net.Conn) {
	clientBuffer := make([]byte, 1024)
	destBuffer := make([]byte, 1024)
	go func() {
		for {
			read, err := c.Connection.Read(clientBuffer)
			if read == 0 {
				log.Logger.Info().Msgf("Zero Byte Read Socks Client closed connection.")
				c.BlockingChannel <- 0
				return
			}
			if err != nil {
				if err != io.EOF {
					fmt.Println(read)
					log.Logger.Error().Msgf("Failed to read from client connection %v", err)
					c.BlockingChannel <- 0
					return
				}
			}
			// refresh client read and write connection whenver we actually read data.
			// example using http connections arent kept alive so we have a way to clean them up.
			c.Connection.SetDeadline(time.Now().Add(time.Second * time.Duration(s.IdleTimeout)))
			log.Logger.Debug().Msgf("Read %d bytes from client connection", read)
			wrote, err := destinationConn.Write(clientBuffer[:read])
			if err != nil {
				log.Logger.Error().Msgf("Failed to forward data to destination server %v", err)
				destinationConn.Close()
				c.BlockingChannel <- 0
				return
			}
			log.Logger.Debug().Msgf("Wrote %d bytes from client bufffer to destination\n", wrote)
		}
	}()
	go func() {
		for {
			read, err := destinationConn.Read(destBuffer)
			if read == 0 {
				log.Logger.Info().Msgf("Zero Byte Read Destination Server Closed connection.")
				destinationConn.Close()
				c.BlockingChannel <- 0
				return
			}
			if err != nil {
				if err != io.EOF {
					log.Logger.Error().Msgf("Failed to read from dest connection %v", err)
					c.BlockingChannel <- 0
					return
				}
			}
			// just incase server goes offline and we arent stuck? reading?
			destinationConn.SetDeadline(time.Now().Add(time.Second * time.Duration(s.IdleTimeout)))
			log.Logger.Debug().Msgf("Read %d bytes from dest connection", read)
			wrote, err := c.Connection.Write(destBuffer[:read])
			if err != nil {
				log.Logger.Error().Msgf("Failed to forward data to socks client %v", err)
				destinationConn.Close()
				c.BlockingChannel <- 0
				return
			}
			log.Logger.Debug().Msgf("Wrote %d bytes from destination server to socks client\n", wrote)
		}
	}()
	<-c.BlockingChannel
	s.RemoveConnection(c.ConnectionId)
	return
}

func (s *Serverv5) SendResponsev4(c SocksConnection, errorType byte, rawData []byte) {
	clientRequest := (*data.Socksv5Requestv4)(unsafe.Pointer(&rawData[0]))
	//destPort := binary.BigEndian.Uint16(clientRequest.PORT[:])
	//destAddressIp := net.IPv4(clientRequest.ADDRESS[0], clientRequest.ADDRESS[1], clientRequest.ADDRESS[2], clientRequest.ADDRESS[3])
	//destString := fmt.Sprintf("%s:%d", destAddressIp.String(), destPort)
	response := data.SocksV5Responsev4{
		Version:     0x5,
		Reply:       errorType,
		AddressType: 0x1,
		Reserved:    0x00,
		ADDRESS:     clientRequest.ADDRESS,
		PORT:        clientRequest.PORT,
	}
	c.Connection.Write(ToBytes(response))
	if errorType != 0x00 {
		s.RemoveConnection(c.ConnectionId)
	}
}

func (s *Serverv5) CheckConnectivityIpv4(rawData []byte) (net.Conn, error) {
	clientRequest := (*data.Socksv5Requestv4)(unsafe.Pointer(&rawData[0]))
	destPort := binary.BigEndian.Uint16(clientRequest.PORT[:])
	destAddressIp := net.IPv4(clientRequest.ADDRESS[0], clientRequest.ADDRESS[1], clientRequest.ADDRESS[2], clientRequest.ADDRESS[3])
	destString := fmt.Sprintf("%s:%d", destAddressIp.String(), destPort)
	c, err := net.Dial("tcp", destString)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Serverv5) HandleClient(c SocksConnection) {
	// first negotiate method with client
	maxSize := unsafe.Sizeof(data.SocksNegotiateRequest{}) + 1
	dataBuffer := make([]byte, maxSize)
	c.Connection.SetReadDeadline(time.Now().Add(time.Second * 15))
	readBytes, err := c.Connection.Read(dataBuffer)
	if err != nil {
		if err != io.EOF {
			log.Logger.Error().Msgf("Connection ERROR: %v Closing client connection", err)
			s.RemoveConnection(c.ConnectionId)
			return
		}
	}
	if readBytes <= 0 {
		log.Logger.Error().Msgf("Not Enough Data Read From Socket Expected %d bytes got %d bytes", maxSize, readBytes)
		s.RemoveConnection(c.ConnectionId)
		return
	}
	log.Logger.Debug().Msgf("Read SOCKS5 Negotiate Request Message: Read %d bytes -> data %s\n", readBytes, hex.EncodeToString(dataBuffer))
	clientRequest := (*data.SocksNegotiateRequest)(unsafe.Pointer(&dataBuffer[0]))
	fmt.Printf("%+v\n", clientRequest)
	if clientRequest.Version != 0x5 {
		log.Logger.Error().Msgf("Socksv5 Negotiate Request Validation Failed ::: INVALID VERSION PROVIDED %v", err)
		s.RemoveConnection(c.ConnectionId)
		return
	}
	if clientRequest.Nmethods == 0x00 {
		log.Logger.Error().Msgf("Socksv5 Negotiate Request Validation Failed ::: NO METHODS PROVIDED %v", err)
		s.RemoveConnection(c.ConnectionId)
		return
	}
	if findMethodNoAuth(clientRequest.Methods) {
		s.HandleNoAuth(c)
		return
	}
	if findMethodGSSAPI(clientRequest.Methods) {
		s.HandleGSSAPI()
		return
	}
	if findMethodBasicAuth(clientRequest.Methods) {
		s.HandleBasicAuth()
		return
	}
	s.HandleNoAcceptableMethods(c)
}

func (s *Serverv5) HandleGSSAPI()    { panic("TODO GSSAPI") }
func (s *Serverv5) HandleBasicAuth() { panic("TODO BASIC AUTH") }

func (s *Serverv5) HandleNoAcceptableMethods(c SocksConnection) {
	// write a no acceptable methods packet
	log.Logger.Debug().Msgf("Client provided no acceptable methods")
	resp := data.SocksNegotiateResponse{
		Method:  0xFF,
		Version: 0x5,
	}
	wrote, err := c.Connection.Write(data.ToBytes(resp))
	if err != nil {
		s.RemoveConnection(c.ConnectionId)
		return
	}
	log.Logger.Debug().Msgf("Wrote FAILED Method Negotiate Response Bytes %d", wrote)
}

func (s *Serverv5) Serve() error {
	s.MaxConnections = 100
	s.IdleTimeout = 120 // seconds
	endpoint := fmt.Sprintf("%s:%d", s.ListeningAddr, s.ListeningPort)
	log.Logger.Info().Msgf("Started Socksv4 Proxy On %s", endpoint)
	s.Listener, s.Error = net.Listen("tcp4", endpoint)
	if s.Error != nil {
		log.Logger.Error().Msgf("Failed to listen on %s %v", endpoint, s.Error)
		return s.Error
	}
	defer s.Listener.Close()
	for {
		if s.CurrentConnections+1 >= s.MaxConnections {
			log.Logger.Error().Msgf("Failed to accept connection, currently at max connection capacity %d.", s.MaxConnections)
			continue
		}
		conn, err := s.Listener.Accept()
		if err != nil {
			log.Logger.Error().Msgf("Failed to accept connection %v", err)
			return err
		}
		log.Logger.Info().Msgf("New Connection From %s", conn.RemoteAddr().String())
		connectionId := fmt.Sprintf("%x", s.CurrentConnections)
		clientConnection := SocksConnection{
			Connection:      conn,
			ConnectionId:    connectionId,
			IsClosed:        false,
			BlockingChannel: make(chan int, 1),
		}
		s.AddConnection(clientConnection)
		go s.HandleClient(clientConnection)
	}
}

func (s *Serverv5) Shutdown(ctx context.Context, cancel context.CancelFunc) error {
	log.Logger.Info().Msgf("Received shutdown request...")
	defer cancel()
	go s.shutdownAllConnections(ctx)
	select {
	case <-s.ShutdownChan:
		log.Logger.Info().Msgf("Shutdown all connections successfully")
		return nil
	case <-ctx.Done():
		log.Logger.Error().Msgf("Shutdown deadline exceeeded.")
		return fmt.Errorf("Shutdown Deadline exceeded.")
	}
}

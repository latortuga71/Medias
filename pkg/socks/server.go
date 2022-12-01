package socks

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/latortuga71/medias/pkg/data"
	"github.com/latortuga71/medias/pkg/log"
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

type Serverv4 struct {
	// need channels to stop the Serverv4 and stuff
	ListeningAddr      string
	Logger             string
	ListeningPort      int
	Listener           net.Listener
	ShutdownChan       chan bool
	Error              error
	Conns              *SocksConnectionList
	IdleTimeout        int // this should be shorter since this is updated everytime data is read.
	ReadTimeout        int // read and write should be pretty long. depending on what is being forwarded.
	WriteTimeout       int
	MaxConnections     int
	CurrentConnections int
}

func NewSocksConnectionsList() *SocksConnectionList {
	return &SocksConnectionList{
		Connections: make(map[string]SocksConnection, 1),
	}
}

func NewServerv4(addr string, port int) *Serverv4 {
	return &Serverv4{
		ListeningAddr: addr,
		ListeningPort: port,
		Listener:      nil,
		ShutdownChan:  make(chan bool),
		Error:         nil,
		Conns:         NewSocksConnectionsList(),
	}

}

func (s *Serverv4) SetMaxConnections(max int) {
	s.MaxConnections = max
}

func (s *Serverv4) AddConnection(c SocksConnection) error {
	s.Conns.Mutex.Lock()
	s.Conns.Connections[c.ConnectionId] = c
	s.CurrentConnections++
	s.Conns.Unlock()
	log.Logger.Debug().Msgf("Added Connection Id %d to connection map", c.ConnectionId)
	return nil
}

func (s *Serverv4) RemoveConnection(id string) error {
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

func validateClientV4Request(data []byte) (bool, error) {
	if data[0] != 0x04 {
		return false, fmt.Errorf("Invalid version provided by client.") // Invalid version
	}
	if data[1] != 0x1 && data[1] != 0x2 {
		return false, fmt.Errorf("Invalid command provided by client.") // invalid command not bind or connect
	}
	return true, nil
}

func (s *Serverv4) HandleSocksV4(c SocksConnection) {
	/// read client request
	maxSize := unsafe.Sizeof(data.SocksRequestMethodV4{}) + 1
	dataBuffer := make([]byte, maxSize)
	responseData := data.ServerResponseMessageV4{}
	// wait max 15 seconds here.
	c.Connection.SetReadDeadline(time.Now().Add(time.Second * 15))
	readBytes, err := c.Connection.Read(dataBuffer)
	if err != nil {
		if err != io.EOF {
			log.Logger.Error().Msgf("Connection ERROR: %v Closing client connection", err)
			s.RemoveConnection(c.ConnectionId)
			return
		}
	}
	log.Logger.Debug().Msgf("Read SOCK4 Request Message: Read %d bytes -> data %s\n", readBytes, hex.EncodeToString(dataBuffer))
	if readBytes <= 0 {
		log.Logger.Error().Msgf("Not Enough Data Read From Socket Expected %d bytes got %d bytes", maxSize, readBytes)
		s.RemoveConnection(c.ConnectionId)
		return
	}
	if ok, err := validateClientV4Request(dataBuffer); !ok {
		log.Logger.Error().Msgf("Socksv4 Request Validation Failed %v", err)
		responseData.Command = 0x5B
		responseData.Version = 0x00
		c.Connection.Write(responseData.ToBytes())
		s.RemoveConnection(c.ConnectionId)
		return
	}
	clientRequest := (*data.SocksRequestMethodV4)(unsafe.Pointer(&dataBuffer[0]))
	responseData.Version = 0x00
	destPort := binary.BigEndian.Uint16(clientRequest.Destination.PORT[:])
	destAddressIp := net.IPv4(clientRequest.Destination.ADDR[0], clientRequest.Destination.ADDR[1], clientRequest.Destination.ADDR[2], clientRequest.Destination.ADDR[3])
	destString := fmt.Sprintf("%s:%d", destAddressIp.String(), destPort)
	log.Logger.Debug().Msgf("Attempting To Connect To Client Requested Destination Server %s", destString)
	destinationConn, err := net.Dial("tcp", destString)
	if err != nil {
		responseData.Command = 0x5B
		c.Connection.Write(responseData.ToBytes())
		s.RemoveConnection(c.ConnectionId)
		log.Logger.Error().Msgf("Failed To Connect To Destination Server %s, Closing Client Connection %v", destString, err)
		return
	}
	log.Logger.Debug().Msgf("Successfully Established Connection To Client Requested Destination Server %s", destString)
	// handle connect command
	if clientRequest.Command == 0x1 {
		responseData.Command = 0x5A // request granted
		responseData.Destination = clientRequest.Destination
		c.Connection.Write(responseData.ToBytes())
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
	if clientRequest.Command == 0x2 {
		// handle bind request
		log.Logger.Error().Msgf("Client Requested BIND Command This Hasnt Been Implemented Yet %v", err)
	}
	log.Logger.Error().Msgf("Invalid Command Recieved Closing Connection. %v", err)
	s.RemoveConnection(c.ConnectionId)
	return
}

func (s *Serverv4) Serve() error {
	s.MaxConnections = 100
	s.IdleTimeout = 120 // seconds
	endpoint := fmt.Sprintf("%s:%d", s.ListeningAddr, s.ListeningPort)
	log.Logger.Info().Msgf("Started Socksv4 Proxy On %s", endpoint)
	s.Listener, s.Error = net.Listen("tcp4", endpoint)
	if s.Error != nil {
		log.Logger.Error().Msgf("Failed to listen on %s %v", endpoint, s.Error)
		return s.Error
	}
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
		go s.HandleSocksV4(clientConnection)
	}
}

func (s *Serverv4) Close() error {
	return nil
}

func (s *Serverv4) shutdownAllConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("shutdownallconnections ran out of time")
			return
		default:
			fmt.Println("Looping over all connections and closing and removing them")
			for k := range s.Conns.Connections {
				// send int on channel
				// only problem is we are not closing the destination connection.
				s.Conns.Connections[k].BlockingChannel <- 0
			}
			log.Logger.Debug().Msgf("Closed all connections...")
			s.Listener.Close()
			log.Logger.Debug().Msgf("Closed listener...")
			time.Sleep(time.Hour)
			s.ShutdownChan <- true
			return
		}
	}
}

func (s *Serverv4) Shutdown(ctx context.Context, cancel context.CancelFunc) error {
	log.Logger.Info().Msgf("Received shutdown request.")
	defer cancel()
	go s.shutdownAllConnections(ctx)
	select {
	case <-s.ShutdownChan:
		log.Logger.Info().Msgf("Shutdown all connections successfully")
		return nil
	case <-ctx.Done():
		log.Logger.Error().Msgf("Shutdown exceeded deadline set. Exiting server anyways....")
		return fmt.Errorf("Deadline exceeded.")
	}
}

/*

func (s *Serverv4) HandleClient(c SocksConnection) error {
	err := s.HandleSocksV4(&c)
	return err
}
*/

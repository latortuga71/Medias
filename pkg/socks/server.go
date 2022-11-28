package socks

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/latortuga71/medias/pkg/data"
)

// Each Connection
type SocksConnection struct {
	Connection      net.Conn
	ConnectionId    string
	BlockingChannel chan bool
}

// AllConnection
type SocksConnectionList struct {
	sync.Mutex
	Connections map[string]SocksConnection
}

type Serverv4 struct {
	// need channels to stop the Serverv4 and stuff
	ListeningAddr string
	ListeningPort int
	Listener      net.Listener
	ShutdownChan  chan bool
	Error         error
	Conns         *SocksConnectionList
	// ReadTimeout ?
	// WriteTimeout ?
	// Max Connections ?
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

func (s *Serverv4) AddConnection(c SocksConnection) error {
	s.Conns.Mutex.Lock()
	s.Conns.Connections[c.ConnectionId] = c
	s.Conns.Unlock()
	return nil
}

func (s *Serverv4) RemoveConnection(id string) error {
	s.Conns.Mutex.Lock()
	delete(s.Conns.Connections, id)
	s.Conns.Unlock()
	return nil
}

func HandleVersionMessage(c *SocksConnection) (*data.VersionIdentifyMessage, error) {
	maxSize := unsafe.Sizeof(data.VersionIdentifyMessage{}) + 1
	dataBuffer := make([]byte, maxSize)
	//tmpBuffer := make([]byte, 10) // read in 10 bytes chunks?
	readBytes, err := c.Connection.Read(dataBuffer)
	if err != nil {
		if err != io.EOF {
			return nil, err
		}
	}
	fmt.Printf("[+] HandleVersionMesssage: Read %d bytes -> data %s\n", readBytes, hex.EncodeToString(dataBuffer))
	versionMsg := (*data.VersionIdentifyMessage)(unsafe.Pointer(&dataBuffer[0]))
	return versionMsg, nil
}

/*
func HandleConnnectCommand(c *SocksConnection, methodData *data.SocksRequestMethodV4) {
	// send response
	// for now just send a response.
	resp := data.ServerResponseMessageIPV4{}
	resp.Version = 0x05
	resp.Reply = 0x00 // 0x00 -> success
	resp.Reserved = 0x00
	resp.AddressType = methodData.ATYP
	resp.Bind.ADDR = methodData.DEST.ADDR
	resp.Bind.PORT = methodData.DEST.PORT
	fmt.Println(resp.ToBytes())
	c.Connection.Write(resp.ToBytes())
	//clientDataToForward := make([]byte, 1024)
	fmt.Println("SENT RESPONSE")
	fmt.Printf("FORWARDING DATA!\n")
	destinationConn, err := net.Dial("tcp", "0.0.0.0:80") // <- destination Serverv4.
	if err != nil {
		log.Fatalf("Line 102\n%v", err)
	}
	// forward traffic.
	go func() {
		t := make([]byte, 1024)
		test := bytes.NewBuffer(t)
		fmt.Println("Blocking read...")
		_, err = c.Connection.Read(t)
		if err != nil {
			fmt.Printf("CLIENT ERR %v\n", err)
			c.Connection.Close()
			c.BlockingChannel <- true
			return // not sure if wee need return
		}
		nBytes, err := io.Copy(destinationConn, test)
		if err != nil {
			fmt.Printf("CLIENT ERR %v\n", err)
			c.Connection.Close()
			c.BlockingChannel <- true
			return // not sure if wee need return
		}
		if nBytes > 0 {
			fmt.Printf("Transferred %d bytes\n", nBytes)
		}
	}()
	go func() {
		// check if connection is open
		// copy bytes
		nBytes, err := io.Copy(c.Connection, destinationConn)
		if err != nil {
			fmt.Printf("Serverv4 ERRR %v\n", err)
			destinationConn.Close()
			c.BlockingChannel <- true
			return
		}
		if nBytes > 0 {
			fmt.Printf("Transferred %d bytes\n", nBytes)
		}
	}()
	fmt.Println("BLOCKING TO KEEP EXCHANGE GOING")
	<-c.BlockingChannel
	fmt.Println("PAST BLOCK!")
}
*/
/*
	func HandleNoAuth(c *SocksConnection) {
		readBuffer := make([]byte, 1024)
		reply := [2]byte{0x05, 0x00}
		c.Connection.Write(reply[:])
		fmt.Printf("[+] Sent Method Selection\n")
		readBytes, err := c.Connection.Read(readBuffer)
		if err != nil {
			log.Fatalf("LINE 152: %v\n", err)
		}
		fmt.Printf("[+] READING RESPONSE BYTES %d\n", readBytes)
		fmt.Println(hex.EncodeToString(readBuffer))
		requestMethod := (*data.SocksRequestMethodV4)(unsafe.Pointer(&readBuffer[0]))
		if requestMethod.Cmd == 0x02 {
			log.Fatal("BIND COMMAND NOT IMPLEMENTED YETs")
		}
		if requestMethod.Cmd == 0x03 {
			log.Fatal("UDP COMMAND NOT IMPLEMENTED YETs")
		}
		if requestMethod.ATYP == 0x3 {
			log.Fatal("DNS NOT IMPLEMENTED YET")
		}
		if requestMethod.ATYP == 0x4 {
			log.Fatal("IPV6 NOT IMPLEMENTED YET")
		}
		if requestMethod.ATYP == 0x1 {
			fmt.Println("IPV4 CLIENT")
		}
		if requestMethod.Cmd == 0x01 {
			fmt.Println("CONNECT COMMAND REQUESTED")
			HandleConnnectCommand(c, requestMethod)
			fmt.Println("HANDELED CONNECT COMMAND")
		}
	}
*/

func (s *Serverv4) HandleSocksV4(c *SocksConnection) error {
	/// read client request
	maxSize := unsafe.Sizeof(data.SocksRequestMethodV4{}) + 1
	dataBuffer := make([]byte, maxSize)
	readBytes, err := c.Connection.Read(dataBuffer)
	if err != nil {
		if err != io.EOF {
			return err
		}
	}
	fmt.Printf("[+] Read SOCK4 Request Message: Read %d bytes -> data %s\n", readBytes, hex.EncodeToString(dataBuffer))
	clientRequest := (*data.SocksRequestMethodV4)(unsafe.Pointer(&dataBuffer[0]))
	// check if request is valid TODO
	//................
	// establish connection to destination
	if clientRequest.Version != 0x4 {
		log.Println("NOT VERSION 4")
		return errors.New("Not Version 4")
	}
	if clientRequest.Command == 0x1 {
		// handle connect command
		responseData := data.ServerResponseMessageV4{}
		responseData.Version = 0x00
		fmt.Printf("%x %d\n", clientRequest.Destination.PORT, clientRequest.Destination.PORT)
		destPort := binary.BigEndian.Uint16(clientRequest.Destination.PORT[:])
		destAddressIp := net.IPv4(clientRequest.Destination.ADDR[0], clientRequest.Destination.ADDR[1], clientRequest.Destination.ADDR[2], clientRequest.Destination.ADDR[3])
		destinationConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", destAddressIp.String(), destPort)) // <- destination Serverv4.
		if err != nil {
			log.Fatalf("Failed to connect to destination send back response packet.\n%v", err)
			responseData.Command = 0x5B // request rejected or failed.
			c.Connection.Write(responseData.ToBytes())
			c.Connection.Close()
			fmt.Println("connection closed!")
			return nil
		}
		// send back response packet.
		responseData.Command = 0x5A // request granted
		responseData.Destination = clientRequest.Destination
		c.Connection.Write(responseData.ToBytes())
		// forward traffic.
		go func() {
			nBytes, err := io.Copy(destinationConn, c.Connection)
			if err != nil {
				fmt.Printf("CLIENT ERR %v\n", err)
				c.Connection.Close()
				c.BlockingChannel <- true
				return // not sure if wee need return
			}
			if nBytes > 0 {
				fmt.Printf("Transferred %d bytes\n", nBytes)
			}
		}()
		go func() {
			// check if connection is open
			// copy bytes
			nBytes, err := io.Copy(c.Connection, destinationConn)
			if err != nil {
				fmt.Printf("Serverv4 ERRR %v\n", err)
				destinationConn.Close()
				c.BlockingChannel <- true
				return
			}
			if nBytes > 0 {
				fmt.Printf("Transferred %d bytes\n", nBytes)
			}
		}()
		fmt.Println("BLOCKING TO KEEP EXCHANGE GOING")
		time.Sleep(time.Second * 10)
		c.Connection.Close()
		s.RemoveConnection(c.ConnectionId)
		fmt.Println("PAST BLOCK!")
		return nil
	}
	if clientRequest.Command == 0x2 {
		// handle bind request
		log.Fatalf("bind not implemented")
	}
	log.Fatalf("invalid command")
	return nil
}

func (s *Serverv4) HandleClient(c SocksConnection) error {
	s.HandleSocksV4(&c)
	return nil
}

func (s *Serverv4) Serve() error {
	connections := 0
	// return errrors from here if something goes wrong.
	s.Listener, s.Error = net.Listen("tcp4", fmt.Sprintf("%s:%d", s.ListeningAddr, s.ListeningPort))
	if s.Error != nil {
		log.Fatalf("SERVE ERRROR %v", s.Error)
	}
	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			log.Fatalf("ACCEPT ERROR %v", err)
		}
		// 5 seconds
		conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		conn.SetWriteDeadline(time.Now().Add(time.Second * 5))
		fmt.Printf("[+] Connection From %s\n", conn.RemoteAddr())
		connectionId := fmt.Sprintf("%x", connections)
		clientConnection := SocksConnection{Connection: conn, ConnectionId: connectionId}
		s.AddConnection(clientConnection)
		connections += 1
		go s.HandleClient(clientConnection)
	}
	return nil
}

func (s *Serverv4) Close() error {
	return nil
}
func (s *Serverv4) Shutdown(ctx context.Context) error {
	return nil
}

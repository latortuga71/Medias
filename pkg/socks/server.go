package socks

import (
	"bytes"
	"context"
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
	BlockingChannel chan bool
}

// AllConnection
type SocksConnectionList struct {
	sync.Mutex
	Connections []SocksConnection
}

type Server struct {
	// need channels to stop the server and stuff
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
		Connections: make([]SocksConnection, 1),
	}
}

func NewServer(addr string, port int) *Server {
	return &Server{
		ListeningAddr: addr,
		ListeningPort: port,
		Listener:      nil,
		ShutdownChan:  make(chan bool),
		Error:         nil,
		Conns:         NewSocksConnectionsList(),
	}

}

func (s *Server) AddConnection(c SocksConnection) error {
	s.Conns.Mutex.Lock()
	s.Conns.Connections = append(s.Conns.Connections, c)
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
	destinationConn, err := net.Dial("tcp", "0.0.0.0:80") // <- destination server.
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
			fmt.Printf("SERVER ERRR %v\n", err)
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

func (s *Server) HandleClient(c SocksConnection) error {
	versionMsg, err := HandleVersionMessage(&c)
	if err != nil {
		s.Error = err
		return err
	}
	switch versionMsg.Version {
	case 5:
		fmt.Println("[+] client wants socks5")
		possibleMethods := versionMsg.ParseMethods()
		if possibleMethods.NoAuthentication {
			HandleNoAuth(&c)
			return nil
		} else {
			log.Fatal("Havent implemented yet.")
		}
	case 4:
		log.Fatal("[+] UNIMPLEMENTED client wants socks4")
	default:
		log.Fatal("[+] invalid version type")
		return errors.New("Invalid Socks Version")
	}
	return nil
}

func (s *Server) Serve() error {
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
		clientConnection := SocksConnection{Connection: conn}
		s.AddConnection(clientConnection)
		go s.HandleClient(clientConnection)
	}
	return nil
}

func (s *Server) Close() error {
	return nil
}
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

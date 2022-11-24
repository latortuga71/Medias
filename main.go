package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"unsafe"

	"github.com/latortuga71/medias/pkg/data"
)

func HandleVersionMessage(conn net.Conn) *data.VersionIdentifyMessage {
	maxSize := unsafe.Sizeof(data.VersionIdentifyMessage{}) + 1
	dataBuffer := make([]byte, maxSize)
	//tmpBuffer := make([]byte, 10) // read in 10 bytes chunks?
	readBytes, err := conn.Read(dataBuffer)
	if err != nil {
		if err != io.EOF {
			log.Fatal(err)
		}
	}
	fmt.Printf("[+] HandleVersionMesssage: Read %d bytes -> data %s\n", readBytes, hex.EncodeToString(dataBuffer))
	versionMsg := (*data.VersionIdentifyMessage)(unsafe.Pointer(&dataBuffer[0]))
	return versionMsg
}
func HandleNoAuth(conn net.Conn) {
	readBuffer := make([]byte, 1024)
	reply := [2]byte{0x05, 0x00}
	conn.Write(reply[:])
	fmt.Printf("[+] Sent Method Selection\n")
	readBytes, err := conn.Read(readBuffer)
	if err != nil {
		log.Fatal(err)
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
		HandleConnnectCommand(conn, requestMethod)
		fmt.Println("HANDELED CONNECT COMMAND")
	}
}

/*
   +----+-----+-------+------+----------+----------+
   |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
   +----+-----+-------+------+----------+----------+
   | 1  |  1  | X'00' |  1   | Variable |    2     |
   +----+-----+-------+------+----------+----------+
*/
func HandleConnnectCommand(incomingConn net.Conn, methodData *data.SocksRequestMethodV4) {
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
	incomingConn.Write(resp.ToBytes())
	//clientDataToForward := make([]byte, 1024)
	fmt.Println("SENT RESPONSE")
	/*
		bytesRead, err := incomingConn.Read(clientDataToForward)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("READ %d BYTES\n", bytesRead)
		fmt.Printf("DATA %s\n", hex.EncodeToString(clientDataToForward))
	*/
	fmt.Printf("FORWARDING DATA!\n")
	destinationConn, err := net.Dial("tcp", "0.0.0.0:80")
	if err != nil {
		log.Fatal(err)
	}
	// forward traffic.
	go func() {
		nBytes, err := io.Copy(destinationConn, incomingConn)
		if err != nil {
			fmt.Printf("CLIENT ERR %v\n", err)
			incomingConn.Close()
			//destinationConn.Close()
			//p.SrcConns[max-1].StopChan <- true
			return
		}
		if nBytes > 0 {
			fmt.Printf("Transferred %d bytes\n", nBytes)
		}
	}()
	go func() {
		// check if connection is open
		// copy bytes
		nBytes, err := io.Copy(incomingConn, destinationConn)
		if err != nil {
			fmt.Printf("SERVER ERRR %v\n", err)
			destinationConn.Close()
			//p.SrcConns[max-1].StopChan <- true
			return
		}
		if nBytes > 0 {
			fmt.Printf("Transferred %d bytes\n", nBytes)
		}
	}()
	fmt.Println("BLOCKING TO KEEP EXCHANGE GOING")
	<-blockChan

}

var blockChan chan bool

func HandleClient(conn net.Conn) {
	// go routine start
	versionMsg := HandleVersionMessage(conn)
	switch versionMsg.Version {
	case 5:
		fmt.Println("[+] client wants socks5")
		possibleMethods := versionMsg.ParseMethods()
		if possibleMethods.NoAuthentication {
			HandleNoAuth(conn)
		} else {
			log.Fatal("Havent implemented yet.")
		}
	case 4:
		log.Fatal("[+] UNIMPLEMENTED client wants socks4")
	default:
		log.Fatal("[+] invalid version type")
		return
	}
}

func main() {
	blockChan = make(chan bool)
	listener, err := net.Listen("tcp4", "0.0.0.0:1080")
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("[+] Connection From %s\n", conn.RemoteAddr())
		go HandleClient(conn)
	}
	//fmt.Printf("[+] Read %d bytes\n", readBytes)
	//fmt.Printf("Data -> %s \n", hex.EncodeToString(dataBuffer))
	//conn.Close()

}

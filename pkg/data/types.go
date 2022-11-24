package data

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	NOAUTH   = 0x00
	GSSAPI   = 0x01
	USERPASS = 0x02
)

type AuthTypes struct {
	NoAuthentication bool
	Gssapi           bool
	UserPass         bool
}

type VersionIdentifyMessage struct {
	Version  byte
	Nmethods byte
	Methods  [255]byte
}

/*
   o  X'00' NO AUTHENTICATION REQUIRED
   o  X'01' GSSAPI
   o  X'02' USERNAME/PASSWORD
*/

func (v *VersionIdentifyMessage) ParseMethods() AuthTypes {
	availableMethods := make([]byte, 0)
	var a AuthTypes
	for x := 0; x < int(v.Nmethods); x++ {
		availableMethods = append(availableMethods, v.Methods[x])
	}
	for _, b := range availableMethods {
		if b == 0x00 {
			fmt.Println("Wants No Auth")
			a.NoAuthentication = true
		}
		if b == 0x01 {
			fmt.Println("Wants GSSAPI")
			a.Gssapi = true
		}
		if b == 0x02 {
			fmt.Println("WANTS user/pass")
			a.UserPass = true
		}
	}
	//fmt.Println(hex.EncodeToString(availableMethods))
	return a
}

/*

   +----+----------+----------+
   |VER | NMETHODS | METHODS  |
   +----+----------+----------+
   | 1  |    1     | 1 to 255 |
   +----+----------+----------+

*/

type MethodSelectionMethod struct {
	Version byte
	Method  byte
}

type DOMAINNAME struct {
	NAME []byte
	PORT [2]byte
}

type IPV4 struct {
	ADDR [4]byte
	PORT [2]byte
}

type IPV6 struct {
	ADDR []byte // dont know size ?
	PORT [2]byte
}

type SocksRequestMethodV4 struct {
	Version byte
	Cmd     byte
	RSV     byte
	ATYP    byte
	DEST    IPV4
}

type ServerResponseMessageIPV4 struct {
	Version     byte
	Reply       byte
	Reserved    byte
	AddressType byte
	Bind        IPV4
}

func (s *ServerResponseMessageIPV4) ToBytes() []byte {
	// converts to big endian
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, s)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return buf.Bytes()
}

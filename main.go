package main

import (
	"fmt"

	"github.com/latortuga71/medias/pkg/socks"
)

func main() {

	srv := socks.NewServerv4("0.0.0.0", 1080)
	srv.Serve()
	fmt.Println("WE GOT TO THE END!")
}

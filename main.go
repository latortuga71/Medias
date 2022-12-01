package main

import (
	"context"
	"fmt"
	"time"

	"github.com/latortuga71/medias/pkg/log"
	"github.com/latortuga71/medias/pkg/socks"
)

func main() {
	log.SetLevelDebug()
	//log.SetLevelInfo()
	srv := socks.NewServerv4("0.0.0.0", 1080)
	go srv.Serve()

	time.Sleep(time.Second * 5)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)

	srv.Shutdown(ctx, cancel)

	fmt.Println("WE GOT TO THE END!")
	fmt.Println("Simulating going back to doing other stuff")
	time.Sleep(time.Second * 5)
	fmt.Println(srv.CurrentConnections)
	time.Sleep(time.Second * 10)
	time.Sleep(time.Hour)

}

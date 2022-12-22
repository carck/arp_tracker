package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/carck/arp_tracker/internal/app"
)

func main() {
	app.InitApp()  // before all
	app.InitMqtt() // optional

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	fmt.Printf("exit with signal: %s\n", <-sigs)
}

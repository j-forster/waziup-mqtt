package main

import (
	"log"

	"github.com/j-forster/mqtt"
	waziup "github.com/j-forster/waziup-mqtt"
	// "net/http"
	//  _ "net/http/pprof"
)


func main() {

	log.Println("Waziup mqtt-server startup..")
	handler := waziup.NewWaziupHandler()
	log.Println("Up and running: Port 1883")
	mqtt.ListenAndServe(":1883", handler)
}
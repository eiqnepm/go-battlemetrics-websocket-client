package main

import (
	"log"
	"os"
	"strings"

	"github.com/eiqnepm/go-battlemetrics-websocket-client/internal/client"
)

func onMessage(msg client.IRTMessage) {
	log.Println(msg)
}

func main() {
	log.SetFlags(log.Flags() | log.LUTC)

	var channels []string
	for _, channel := range strings.Split(os.Getenv("SERVERS"), ",") {
		channels = append(channels, "server:events:"+channel)
	}

	client.Client{
		Channels: channels,
	}.Read(onMessage)
}

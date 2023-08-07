package main

import (
	"gobmclient/internal/client"
	"log"
	"os"
	"strings"
	"time"
)

func onMessage(msg client.IRTMessage) {
	log.Println(msg)
}

func main() {
	for _, sid := range strings.Split(os.Getenv("SERVERS"), ",") {
		client.Join("server:events:" + sid)
	}
	filter := client.IActivityFilter{
		TagTypeMode: "and",
		Types: client.IActivityFilterTypes{
			Whitelist: []string{"addPlayer", "removePlayer"},
		},
	}
	client.Filter("ACTIVITY", filter)
	client.HandleFunc(onMessage)
	client.Listen(5 * time.Minute)
}

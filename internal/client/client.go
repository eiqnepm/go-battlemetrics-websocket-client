package client

import (
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"golang.org/x/exp/slices"
)

var channels []string

func Join(channel string) {
	if slices.Contains(channels, channel) {
		return
	}
	channels = append(channels, channel)
}

type IActivityFilterTags struct {
	Whitelist []string `json:"whitelist,omitempty"`
	Blacklist []string `json:"blacklist,omitempty"`
}

type IActivityFilterTypes struct {
	Whitelist []string `json:"whitelist,omitempty"`
	Blacklist []string `json:"blacklist,omitempty"`
}

type IActivityFilter struct {
	TagTypeMode string
	Tags        IActivityFilterTags  `json:"tags"`
	Types       IActivityFilterTypes `json:"types"`
}

var filters = make(map[string]IActivityFilter)

func Filter(t string, filter IActivityFilter) {
	filters[t] = filter
}

type IRTMessage struct {
	I string `json:"i"`
	T string `json:"t"`
	C string `json:"c,omitempty"`
	P any    `json:"p,omitempty"`
}

var handleFunc func(msg IRTMessage)

func HandleFunc(handler func(msg IRTMessage)) {
	handleFunc = handler
}

var (
	lastPing      time.Time
	lastMessage   string
	retryDelay    time.Duration
	lastConnected time.Time
)

func Listen(replayMaxTime time.Duration) {
	for {
		func() {
			conn, _, err := websocket.DefaultDialer.Dial("wss://ws.battlemetrics.com", nil)
			if err != nil {
				log.Println("dial:", err)
				return
			}
			defer conn.Close()
			retryDelay = 0
			prevLastConnected := lastConnected
			lastConnected = time.Now()

			done := make(chan interface{})
			go func() {
				defer close(done)
				for {
					var msg IRTMessage
					err := conn.ReadJSON(&msg)
					if err != nil {
						log.Println("read:", err)
						return
					}
					lastPing = time.Now()

					if msg.T == "ack" {
						continue
					}
					lastMessage = msg.I

					if handleFunc == nil {
						continue
					}
					go handleFunc(msg)
				}
			}()

			sendQueue := make(chan IRTMessage)
			go func() {
				for {
					select {
					case msg := <-sendQueue:
						err := conn.WriteJSON(msg)
						if err != nil {
							log.Println("write:", err)
						}
					case <-done:
						return
					}
				}
			}()
			for t, filter := range filters {
				sendQueue <- IRTMessage{
					I: uuid.New().String(),
					T: "filter",
					P: map[string]interface{}{
						"type":   t,
						"filter": filter,
					},
				}
			}
			sendQueue <- IRTMessage{
				I: uuid.New().String(),
				T: "join",
				P: channels,
			}
			if lastMessage != "" && !prevLastConnected.IsZero() && time.Since(prevLastConnected) < replayMaxTime {
				log.Println("Will replay")
				sendQueue <- IRTMessage{
					I: uuid.New().String(),
					T: "replay",
					P: map[string]interface{}{
						"channels": channels,
						"start":    lastMessage,
					},
				}
			} else {
				lastMessage := "never"
				if !prevLastConnected.IsZero() {
					lastMessage = time.Since(prevLastConnected).String() + " ago"
				}
				log.Println("No replay. Last message was too long ago, or no message to replay from. Last connected: " + lastMessage)
			}

			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			go func() {
				for {
					select {
					case <-ticker.C:
						if time.Now().After(lastPing.Add(1 * time.Minute)) {
							log.Println("no message received")
							close(done)
							return
						}

						sendQueue <- IRTMessage{
							I: uuid.New().String(),
							T: "ping",
						}
					case <-done:
						return
					}
				}
			}()

			<-done
		}()

		time.Sleep(retryDelay)
		retryDelay += time.Duration(rand.Intn(6)+5) * time.Second
		if retryDelay > time.Duration(60*time.Second) {
			retryDelay = 60 * time.Second
		}
	}
}

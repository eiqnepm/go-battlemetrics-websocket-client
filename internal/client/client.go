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
	retryDelay      time.Duration
	lastPing        time.Time
	lastMessageTime time.Time
	lastMessage     string
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
					messageTime := time.Now()
					lastPing = messageTime
					if msg.T == "ack" {
						continue
					}
					lastMessageTime = messageTime
					lastMessage = msg.I

					if handleFunc == nil {
						continue
					}
					go handleFunc(msg)
				}
			}()

			sendQueue := make(chan IRTMessage, 1)
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

			if !lastMessageTime.IsZero() && time.Since(lastMessageTime) < replayMaxTime {
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
				if !lastMessageTime.IsZero() {
					lastMessage = time.Since(lastMessageTime).String() + " ago"
				}
				log.Println("No replay. Last message was too long ago, or no message to replay from. Last message: " + lastMessage)
			}

			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			go func() {
				for {
					select {
					case <-ticker.C:
						if time.Now().After(lastPing.Add(1 * time.Minute)) {
							log.Println("Last activity more than a minute ago. Closing connection.")
							conn.Close()
							return
						}

						if len(sendQueue) != 0 {
							continue
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

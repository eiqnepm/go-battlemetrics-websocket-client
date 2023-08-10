package client

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type IRTMessage struct {
	I string `json:"i"`
	T string `json:"t"`
	C string `json:"c,omitempty"`
	P any    `json:"p,omitempty"`
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

type Client struct {
	Filters  map[string]IActivityFilter
	Channels []string
}

func (c Client) Read(handleFunc func(msg IRTMessage)) {
	var (
		retryDelay  time.Duration
		lastMessage struct {
			id   string
			time time.Time
		}
	)

	for {
		func() {
			conn, _, err := websocket.DefaultDialer.Dial("wss://ws.battlemetrics.com", nil)
			if err != nil {
				log.Println("dial:", err)
				return
			}

			defer conn.Close()
			retryDelay = 0

			for t, filter := range c.Filters {
				if err := conn.WriteJSON(IRTMessage{
					I: uuid.New().String(),
					T: "filter",
					P: map[string]interface{}{
						"type":   t,
						"filter": filter,
					},
				}); err != nil {
					log.Println("write:", err)
					return
				}
			}

			if err := conn.WriteJSON(IRTMessage{
				I: uuid.New().String(),
				T: "join",
				P: c.Channels,
			}); err != nil {
				log.Println("write:", err)
				return
			}

			if lastMessage.id != "" {
				log.Println("replaying from " + lastMessage.time.UTC().Format(time.DateTime))
				if err := conn.WriteJSON(IRTMessage{
					I: uuid.New().String(),
					T: "replay",
					P: map[string]interface{}{
						"channels": c.Channels,
						"start":    lastMessage.id,
					},
				}); err != nil {
					log.Println("write:", err)
					return
				}
			}

			interrupt := make(chan interface{})
			var lastPing struct {
				m    sync.RWMutex
				time time.Time
			}

			go func() {
				defer close(interrupt)
				for {
					var msg IRTMessage
					if err := conn.ReadJSON(&msg); err != nil {
						log.Println("read:", err)
						return
					}

					p := func() time.Time {
						lastPing.m.Lock()
						defer lastPing.m.Unlock()
						lastPing.time = time.Now()
						return lastPing.time
					}()

					if msg.T == "ack" {
						continue
					}

					lastMessage.time = p
					lastMessage.id = msg.I

					go handleFunc(msg)
				}
			}()

			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			go func() {
				for {
					select {
					case <-ticker.C:
						p := func() time.Time {
							lastPing.m.RLock()
							defer lastPing.m.RUnlock()
							return lastPing.time
						}()

						if time.Now().After(p.Add(1 * time.Minute)) {
							log.Println("last ping " + p.UTC().Format(time.DateTime))
							conn.Close()
							return
						}

						conn.WriteJSON(IRTMessage{
							I: uuid.New().String(),
							T: "ping",
						})
					case <-interrupt:
						return
					}
				}
			}()

			<-interrupt
		}()

		time.Sleep(retryDelay)
		retryDelay += time.Duration(rand.Intn(6)+5) * time.Second
		if retryDelay > time.Duration(60*time.Second) {
			retryDelay = 60 * time.Second
		}
	}
}

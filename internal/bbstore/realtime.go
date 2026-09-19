package bbstore

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type subscribeMessage struct {
	Type   string            `json:"type"`
	Target map[string]string `json:"target"`
}

type changedMessage struct {
	Type    string   `json:"type"`
	Entity  string   `json:"entity"`
	ID      string   `json:"id"`
	Changes []string `json:"changes"`
}

func realtimeURL(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/ws"
	return parsed.String()
}

// Watch keeps the cache current from bb's change feed, so the board reacts as
// the app does instead of waiting out a poll interval. It reconnects with
// backoff and refreshes on every reconnect, because changes that landed while
// the socket was down are not replayed.
func (s *Store) Watch(stop <-chan struct{}, onChange func()) {
	endpoint := realtimeURL(s.client.BaseURL)
	if endpoint == "" {
		return
	}

	delay := time.Second
	for {
		select {
		case <-stop:
			return
		default:
		}

		conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
		if err != nil {
			select {
			case <-stop:
				return
			case <-time.After(delay):
			}
			if delay < 30*time.Second {
				delay = time.Duration(float64(delay) * 1.5)
			}
			continue
		}
		delay = time.Second

		for _, target := range []map[string]string{
			{"kind": "thread-list"},
			{"kind": "project-list"},
			{"kind": "system"},
		} {
			_ = conn.WriteJSON(subscribeMessage{Type: "subscribe", Target: target})
		}

		_ = s.Refresh()
		onChange()

		go func() {
			<-stop
			_ = conn.Close()
		}()

		for {
			_, raw, readErr := conn.ReadMessage()
			if readErr != nil {
				break
			}
			var message changedMessage
			if json.Unmarshal(raw, &message) != nil || message.Type != "changed" {
				continue
			}

			if message.Entity == "thread" || message.Entity == "project" {
				_ = s.Refresh()
				for _, change := range message.Changes {
					if change == "events-appended" || change == "interactions-changed" {
						s.RefreshOpenLogs()
						break
					}
				}
				onChange()
			}
		}
		_ = conn.Close()

		select {
		case <-stop:
			return
		case <-time.After(time.Second):
		}
	}
}

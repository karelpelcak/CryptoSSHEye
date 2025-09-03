package websocket

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const binanceWS = "wss://stream.binance.com:9443/ws/btcusdt@miniTicker"

func WsReader(ctx context.Context, out chan<- float64, logf func(string, ...any)) {
	defer close(out)
	dialer := websocket.Dialer{}
	retry := 0

	for {
		c, _, err := dialer.DialContext(ctx, binanceWS, nil)
		if err != nil {
			retry++
			backoff := time.Duration(min(30, 1<<min(6, retry))) * time.Second
			logf("WS connect failed: %v — retrying in %s", err, backoff)
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return
			}
		}
		retry = 0
		logf("Connected to Binance stream")

		c.SetReadLimit(1 << 20)
		_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
		c.SetPongHandler(func(string) error {
			_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
			return nil
		})

		go func() {
			ticker := time.NewTicker(45 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = c.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
				}
			}
		}()

		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				logf("WS read error: %v", err)
				_ = c.Close()
				break
			}

			var tm struct {
				C string `json:"c"`
			}
			if err := json.Unmarshal(data, &tm); err != nil {
				continue
			}
			v, err := strconv.ParseFloat(tm.C, 64)
			if err != nil {
				continue
			}

			select {
			case out <- v:
			default:
			case <-ctx.Done():
				_ = c.Close()
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

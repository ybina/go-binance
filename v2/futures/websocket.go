package futures

import (
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// WsHandler handle raw websocket message
type WsHandler func(message []byte)

// ErrHandler handles errors
type ErrHandler func(err error)

// WsConfig webservice configuration
type WsConfig struct {
	Endpoint string
}

func newWsConfig(endpoint string) *WsConfig {
	return &WsConfig{
		Endpoint: endpoint,
	}
}

var wsServe = func(cfg *WsConfig, handler WsHandler, errHandler ErrHandler) (doneC, stopC chan struct{}, err error) {
	isStop := false

	doneC = make(chan struct{})
	stopC = make(chan struct{})
	var conn *websocket.Conn
	conn, err = newDialer(cfg, true)
	go func() {
		// This function will exit either on error from
		// websocket.Conn.ReadMessage or when the stopC channel is
		// closed by the client.
		defer close(doneC)
		// Wait for the stopC channel to be closed.  We do that in a
		// separate goroutine because ReadMessage is a blocking
		// operation.
		silent := false
		go func() {
			select {
			case <-stopC:
				silent = true
			case <-doneC:
			}
			isStop = true
			conn.Close()
			return
		}()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("conn.ReadMessage error:%v\n", err)
				if !silent {
					errHandler(err)
					continue
				}
				if isStop {
					return
				} else {
				nextDial:
					err = conn.Close()
					if err != nil {
						log.Printf("close conn fail:%v\n", err)
					}
					time.Sleep(time.Second)
					conn, err = newDialer(cfg, isProxy)
					if err != nil {
						log.Printf("newDialer err:%v\n", err)
						goto nextDial
					} else {
						log.Printf("redial ws succ, start continue read msg\n")
						continue
					}
				}

			}
			handler(message)
		}
	}()
	return
}

func newDialer(cfg *WsConfig, isProxy bool) (*websocket.Conn, error) {
	d := websocket.DefaultDialer

	if isProxy {
		proxy := func(_ *http.Request) (*url.URL, error) {
			return url.Parse(pUrl)
		}
		d.Proxy = proxy
	}
	c, _, err := websocket.DefaultDialer.Dial(cfg.Endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(655350)
	if WebsocketKeepalive {
		keepAlive(c, WebsocketTimeout)
	}
	return c, nil
}

func keepAlive(c *websocket.Conn, timeout time.Duration) {
	ticker := time.NewTicker(timeout)

	lastResponse := time.Now()
	c.SetPongHandler(func(msg string) error {
		lastResponse = time.Now()
		return nil
	})

	go func() {
		defer ticker.Stop()
		for {
			deadline := time.Now().Add(10 * time.Second)
			err := c.WriteControl(websocket.PingMessage, []byte{}, deadline)
			if err != nil {
				return
			}
			<-ticker.C
			if time.Since(lastResponse) > timeout {
				c.Close()
				return
			}
		}
	}()
}

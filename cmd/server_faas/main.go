package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/jarvisgally/v2simple/proxy/faas"
	"github.com/tencentyun/scf-go-lib/cloudfunction"
)

type TunnelRequest = faas.TunnelRequest
type TunnelResponse = faas.TunnelResponse

type TunnelClient struct {
	connectionMap sync.Map
}

type Connection struct {
	connectionId string
	eof          bool
	target       string
	conn         net.Conn
	lastActive   time.Time
}

func Marshal2String(a *TunnelResponse) string {
	log.Printf("[response]: <%s>, eof: %v, code: %d, data: %d", a.ConnectionId, a.Eof, a.Code, len(a.Data))
	data, _ := json.Marshal(a)
	return string(data)
}

func (c *TunnelClient) tunnel(ctx context.Context, event TunnelRequest) (string, error) {
	ret := &TunnelResponse{
		ConnectionId: event.ConnectionId,
	}
	log.Printf("[request]: <%s>, %s ==> %s, data: %d", event.ConnectionId, event.Action, event.Target, len(event.Data))
	_, ok := c.connectionMap.Load(event.ConnectionId)

	if event.Action == "connect" || !ok {
		conn, err := net.Dial("tcp", event.Target)
		ret.Action = "connect"
		if err != nil {
			log.Println("connect err: ", err)
			ret.Code = 500
			ret.Message = err.Error()
		} else {
			c.connectionMap.Store(event.ConnectionId, &Connection{
				connectionId: event.ConnectionId,
				target:       event.Target,
				conn:         conn,
				lastActive:   time.Now(),
			})
		}
		return Marshal2String(ret), nil
	} else if event.Action == "read" {
		ret.Action = "read"
		if saved, ok := c.connectionMap.Load(event.ConnectionId); !ok {
			ret.Code = 404
			ret.Message = "missing connection"
			return Marshal2String(ret), nil
		} else {
			conn := saved.(*Connection)
			conn.lastActive = time.Now()
			b := make([]byte, 1024)
			n, err := conn.conn.Read(b)
			ret.Data = b[:n]
			if err == io.EOF {
				conn.eof = true
				ret.Eof = true
			} else if err != nil {
				ret.Code = 501
				ret.Message = err.Error()
			} else {

			}
			return Marshal2String(ret), nil
		}
	} else if event.Action == "write" {
		ret.Action = "write"
		if saved, ok := c.connectionMap.Load(event.ConnectionId); !ok {
			ret.Code = 404
			ret.Message = "missing connection"
			return Marshal2String(ret), nil
		} else {
			conn := saved.(*Connection)
			conn.lastActive = time.Now()
			_, err := conn.conn.Write(event.Data)
			if err == io.EOF {
				conn.eof = true
				ret.Eof = true
			} else if err != nil {
				ret.Code = 502
				ret.Message = err.Error()
			} else {

			}
			return Marshal2String(ret), nil
		}
	} else {
		return "{}", errors.New("invalid action")
	}
}

func main() {
	c := &TunnelClient{}
	cloudfunction.Start(c.tunnel)
}

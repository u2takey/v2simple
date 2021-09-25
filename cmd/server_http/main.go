package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jarvisgally/v2simple/proxy/faas"
	"github.com/sirupsen/logrus"
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

func (c *TunnelClient) tunnel(ctx context.Context, event *TunnelRequest) (*TunnelResponse, error) {
	ret := &TunnelResponse{
		ConnectionId: event.ConnectionId,
	}
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
		return ret, nil
	} else if event.Action == "read" {
		ret.Action = "read"
		if saved, ok := c.connectionMap.Load(event.ConnectionId); !ok {
			ret.Code = 404
			ret.Message = "missing connection"
			return ret, nil
		} else {
			conn := saved.(*Connection)
			conn.lastActive = time.Now()
			b := make([]byte, 1024*10)
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
			return ret, nil
		}
	} else if event.Action == "write" {
		ret.Action = "write"
		if saved, ok := c.connectionMap.Load(event.ConnectionId); !ok {
			ret.Code = 404
			ret.Message = "missing connection"
			return ret, nil
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
			return ret, nil
		}
	} else {
		return nil, errors.New("invalid action")
	}
}

func main() {
	r := gin.Default()
	client := &TunnelClient{}
	r.POST("/", func(c *gin.Context) {
		req := &TunnelRequest{}
		_ = c.BindJSON(req)
		logrus.Printf("[request]: <%s>, %s ==> %s, data: %d", req.ConnectionId, req.Action, req.Target, len(req.Data))
		resp, err := client.tunnel(c, req)
		if err != nil {
			c.JSON(400, gin.H{})
			return
		}
		logrus.Printf("[response]: <%s>, eof: %v, code: %d, data: %d", resp.ConnectionId, resp.Eof, resp.Code, len(resp.Data))
		c.JSON(200, resp)
	})
	log.Fatal(r.Run(":8081")) // listen and serve on 0.0.0.0:8080
}

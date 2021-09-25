package http

import (
	"encoding/json"
	"io"
	"math/rand"
	"net"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jarvisgally/v2simple/proxy"
)

const Name = "http"

func init() {
	proxy.RegisterClient(Name, NewHttpClient)
	rand.Seed(time.Now().UnixNano())
}

type HttpClient struct {
	client *resty.Client
	addr   string
}

func Marshal2String(a interface{}) string {
	data, _ := json.Marshal(a)
	return string(data)
}

func NewHttpClient(url *url.URL) (proxy.Client, error) {
	return &HttpClient{
		client: resty.New(),
		addr:   url.String(),
	}, nil
}

func (c *HttpClient) Name() string { return Name }

func (c *HttpClient) Addr() string { return c.addr }

func (c *HttpClient) Handshake(_ net.Conn, target string) (io.ReadWriter, error) {
	conn := &httpConnection{
		client:       c,
		target:       target,
		connectionId: RandStringRunes(8),
	}
	return conn, conn.Connect()
}

func (c *HttpClient) post(r *TunnelRequest) (*TunnelResponse, error) {
	ret := &TunnelResponse{}
	_, err := c.client.NewRequest().SetResult(ret).SetBody(r).SetHeader("Content-Type", "application/json").Post(c.addr)
	return ret, err
}

type TunnelRequest struct {
	Target       string
	Action       string // create, read, write
	Data         []byte
	ConnectionId string
}

type TunnelResponse struct {
	Target       string
	Action       string
	Data         []byte
	ConnectionId string
	Eof          bool
	Code         int
	Message      string
}

type httpConnection struct {
	client       *HttpClient
	target       string
	readBuffer   []byte
	writeBuffer  []byte
	connectionId string
	eof          bool
	lastWrite    time.Time
}

func (c *httpConnection) Connect() (err error) {
	_, err = c.client.post(&TunnelRequest{
		Target:       c.target,
		Action:       "connect",
		ConnectionId: c.connectionId,
	})
	return err
}

func (c *httpConnection) Read(p []byte) (n int, err error) {
	if c.eof {
		return 0, io.EOF
	}
	if len(c.readBuffer) == 0 {
		resp, err := c.client.post(&TunnelRequest{
			Target:       c.target,
			Action:       "read",
			ConnectionId: c.connectionId,
		})
		if err != nil {
			return 0, err
		}
		c.readBuffer = append(c.readBuffer, resp.Data...)
		if resp.Eof {
			c.eof = true
		}
	}

	n = copy(p, c.readBuffer)
	c.readBuffer = c.readBuffer[:len(c.readBuffer)-n]
	return n, nil
}

func (c *httpConnection) Write(p []byte) (n int, err error) {
	c.writeBuffer = append(c.writeBuffer, p...)
	if len(c.writeBuffer) > 1024 || time.Now().Sub(c.lastWrite) > time.Millisecond*100 {
		resp, err := c.client.post(&TunnelRequest{
			Target:       c.target,
			Action:       "write",
			Data:         c.writeBuffer,
			ConnectionId: c.connectionId,
		})
		if err != nil {
			return 0, err
		}
		_ = resp
		c.writeBuffer = c.writeBuffer[:0]
	}
	return len(p), nil
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

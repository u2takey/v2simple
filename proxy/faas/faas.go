package faas

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jarvisgally/v2simple/proxy"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/regions"
	scf "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/scf/v20180416"
)

const Name = "faas"

func init() {
	proxy.RegisterClient(Name, NewFaasClient)
	rand.Seed(time.Now().UnixNano())
}

type FaasClient struct {
	client       *scf.Client
	functionName string
}

func Marshal2String(a interface{}) string {
	data, _ := json.Marshal(a)
	return string(data)
}

func NewFaasClient(url *url.URL) (proxy.Client, error) {
	credential := common.NewCredential("xx", "yy")
	region := regions.SiliconValley
	if r := url.Query().Get("region"); r != "" {
		region = r
	}
	client, err := scf.NewClient(credential, region, profile.NewClientProfile())
	if err != nil {
		return nil, err
	}
	return &FaasClient{
		client:       client,
		functionName: url.Hostname(),
	}, nil
}

func (c *FaasClient) Name() string { return Name }

func (c *FaasClient) Addr() string { return c.functionName }

func (c *FaasClient) Handshake(_ net.Conn, target string) (io.ReadWriter, error) {
	conn := &faasConnection{
		client:       c,
		target:       target,
		connectionId: RandStringRunes(8),
	}
	return conn, conn.Connect()
}

func (c *FaasClient) post(r *TunnelRequest) (*TunnelResponse, error) {
	data, _ := json.Marshal(r)
	dataStr := string(data)
	req := scf.NewInvokeFunctionRequest()
	req.FunctionName = &c.functionName
	req.Event = &dataStr
	if resp, err := c.client.InvokeFunction(req); err != nil {
		return nil, err
	} else {
		ret := &TunnelResponse{}
		dataStr, _ = strconv.Unquote(*resp.Response.Result.RetMsg)
		log.Printf("call faas with request: %v, response: %v", Marshal2String(r), dataStr)
		err = json.Unmarshal([]byte(dataStr), ret)
		if err != nil {
			return nil, err
		}
		if ret.Code != 0 {
			return nil, fmt.Errorf("post faas got error code: %d", ret.Code)
		}
		return ret, nil
	}
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

type faasConnection struct {
	client       *FaasClient
	target       string
	readBuffer   []byte
	writeBuffer  []byte
	connectionId string
	eof          bool
	lastWrite    time.Time
}

func (c *faasConnection) Connect() (err error) {
	_, err = c.client.post(&TunnelRequest{
		Target:       c.target,
		Action:       "connect",
		ConnectionId: c.connectionId,
	})
	return err
}

func (c *faasConnection) Read(p []byte) (n int, err error) {
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

func (c *faasConnection) Write(p []byte) (n int, err error) {
	c.writeBuffer = append(c.writeBuffer, p...)
	//if len(c.writeBuffer) > 1024 || time.Now().Sub(c.lastWrite) > time.Millisecond*100 {
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
	//}
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

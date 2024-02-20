package node

import (
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"golang.org/x/net/websocket"
	"net/http"
	"time"
)

type TokenAuth struct {
	Token  string
	Secret string
}

type WsClient struct {
	Origin      string
	Addr        string
	Path        string
	auth        TokenAuth
	conn        *websocket.Conn
	AuthCall    func() (string, string, error)
	ReceiveCall func(message []byte, err error) (interface{}, error) // 如响应数据为nil,则不回复服务端
}

type Ping struct {
	HealthCheck string `json:"healthCheck"`
}

func authReq(path string, requestObj interface{}, secret string, encrypted ...bool) ([]byte, error) {
	if len(path) == 0 || requestObj == nil {
		return nil, ex.Throw{Msg: "params invalid"}
	}
	jsonData, err := utils.JsonMarshal(requestObj)
	if err != nil {
		return nil, ex.Throw{Msg: "request data JsonMarshal invalid"}
	}
	jsonBody := &JsonResp{
		Code:  http.StatusOK,
		Data:  jsonData,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  0,
	}
	if len(encrypted) > 0 && encrypted[0] {
		d, err := utils.AesEncrypt(jsonBody.Data.([]byte), secret, utils.AddStr(jsonBody.Nonce, jsonBody.Time))
		if err != nil {
			return nil, ex.Throw{Msg: "request data AES encrypt failed"}
		}
		jsonBody.Data = d
		jsonBody.Plan = 1
	} else {
		d := utils.Base64Encode(jsonBody.Data.([]byte))
		jsonBody.Data = d
	}
	jsonBody.Sign = utils.HMAC_SHA256(utils.AddStr(path, jsonBody.Data.(string), jsonBody.Nonce, jsonBody.Time, jsonBody.Plan), secret, true)
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return nil, ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	return bytesData, nil
}

func authRes(client *WsClient, respBytes []byte) ([]byte, error) {
	if len(respBytes) == 0 {
		return nil, ex.Throw{Msg: "message is nil"}
	}
	respData := &JsonResp{
		Code:    utils.GetJsonInt(respBytes, "c"),
		Message: utils.GetJsonString(respBytes, "m"),
		Data:    utils.GetJsonString(respBytes, "d"),
		Nonce:   utils.GetJsonString(respBytes, "n"),
		Time:    int64(utils.GetJsonInt(respBytes, "t")),
		Plan:    int64(utils.GetJsonInt(respBytes, "p")),
		Sign:    utils.GetJsonString(respBytes, "s"),
	}
	if respData.Code != 200 {
		if respData.Code > 0 {
			return nil, ex.Throw{Code: respData.Code, Msg: respData.Message}
		}
		return nil, ex.Throw{Msg: respData.Message}
	}
	validSign := utils.HMAC_SHA256(utils.AddStr(client.Path, respData.Data, respData.Nonce, respData.Time, respData.Plan), client.auth.Secret, true)
	if validSign != respData.Sign {
		return nil, ex.Throw{Msg: "post response sign verify invalid"}
	}
	var err error
	var dec []byte
	if respData.Plan == 0 {
		dec = utils.Base64Decode(respData.Data)
	} else if respData.Plan == 1 {
		dec, err = utils.AesDecrypt(respData.Data.(string), client.auth.Secret, utils.AddStr(respData.Nonce, respData.Time))
		if err != nil {
			return nil, ex.Throw{Msg: "post response data AES decrypt failed"}
		}
	} else {
		return nil, ex.Throw{Msg: "response sign plan invalid"}
	}
	return dec, nil
}

func (client *WsClient) StartWebsocket(n ...int) {
	for {
		if err := client.initClient(); err != nil {
			zlog.Error("websocket client error", 0, zlog.AddError(err))
		}
		restart := time.Duration(10)
		if len(n) > 0 && n[0] > 0 {
			restart = time.Duration(n[0])
		}
		time.Sleep(restart * time.Second)
	}
}

func (client *WsClient) Ready() bool {
	return client.conn != nil && len(client.auth.Secret) > 0
}

func (client *WsClient) initClient() error {
	if len(client.Addr) == 0 {
		return utils.Error("client addr is nil")
	}

	if len(client.Path) == 0 {
		return utils.Error("client path is nil")
	}

	if len(client.Origin) == 0 {
		return utils.Error("client origin is nil")
	}

	if client.AuthCall == nil {
		return utils.Error("client auth call is nil")
	}

	if client.ReceiveCall == nil {
		return utils.Error("client receive call is nil")
	}

	// 创建 WebSocket 连接
	config, err := websocket.NewConfig(client.Addr+client.Path, client.Origin)
	if err != nil {
		return err
	}

	token, secret, err := client.AuthCall()
	if err != nil {
		return err
	}

	if len(token) == 0 || len(secret) == 0 {
		return utils.Error("token/secret invalid")
	}

	// 设置 JWT 头部
	config.Header.Add("Authorization", token)

	// 建立 WebSocket 连接
	ws, err := websocket.DialConfig(config)
	if err != nil {
		return err
	}
	defer closeConn(ws)

	client.conn = ws
	client.auth = TokenAuth{Token: token, Secret: secret}

	zlog.Info("websocket connect success", 0, zlog.String("url", client.Addr+client.Path))

	go client.ping()

	return client.receive()
}

// receive 读取服务端消息
func (client *WsClient) receive() error {
	for {
		var message []byte
		if err := websocket.Message.Receive(client.conn, &message); err != nil {
			return err
		}
		res, err := authRes(client, message)
		if err != nil {
			zlog.Error("websocket receive parse error", 0, zlog.AddError(err))
			continue
		}
		reply, err := client.ReceiveCall(res, err)
		if err != nil {
			zlog.Error("websocket receive call error", 0, zlog.AddError(err))
		}
		if err := client.SendMessage(reply); err != nil {
			break
		}
	}
	return nil
}

func (client *WsClient) SendMessage(reply interface{}) error {
	if reply == nil {
		return nil
	}
	if !client.Ready() {
		zlog.Warn("client not ready", 0)
		return nil
	}
	data, err := authReq(client.Path, reply, client.auth.Secret)
	if err != nil {
		zlog.Error("websocket receive reply create error", 0, zlog.AddError(err))
		return nil
	}
	if err := websocket.Message.Send(client.conn, data); err != nil {
		zlog.Error("websocket client reply error", 0, zlog.AddError(err))
		return err
	}
	return nil
}

// ping 持续心跳包
func (client *WsClient) ping() {
	for {
		ping := Ping{
			HealthCheck: pingCmd,
		}
		data, _ := authReq(client.Path, &ping, client.auth.Secret)
		if err := websocket.Message.Send(client.conn, data); err != nil {
			zlog.Error("websocket client ping error", 0, zlog.AddError(err))
			break
		}
		time.Sleep(pingTime / 2 * time.Second)
	}
}

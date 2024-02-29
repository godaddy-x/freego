package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"testing"
)

func newClient() {

	//rand.Seed(time.Now().UnixNano())
	//randomNumber := rand.Intn(3000) // 生成0到9的随机整数
	//if randomNumber < 100 {
	//	randomNumber = randomNumber + 100
	//}
	//time.Sleep(time.Duration(randomNumber) * time.Millisecond)

	authCall := func() (string, string, error) {
		config := jwt.JwtConfig{
			TokenTyp: jwt.JWT,
			TokenAlg: jwt.HS256,
			TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
			TokenExp: jwt.TWO_WEEK,
		}
		subject := jwt.Subject{}
		jwtToken := subject.Create(utils.NextSID()).Dev("APP").Generate(config)
		jwtSecret := jwt.GetTokenSecret(jwtToken, config.TokenKey)
		//fmt.Println(jwtToken)
		//fmt.Println(jwtSecret)
		//jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzYzMDI4MjgyNDcyNjYwOTkyIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MTAzODMyNzIsImRldiI6IkFQUCIsImp0aSI6IlEyRW9sWVNyM29GR0JiNWRqeHlsZ1E9PSIsImV4dCI6IiJ9.8SZskvU5vbJ3N0jGT1V3dtFk7dRpjNzqK/NKbmvvxHk="
		//jwtSecret := "mRYCvKf8iqYhTqtHy*kT^j#lKJHd4YPy0QE9xc9H#lK!ZC@diQQdHdVvwnj2hms="
		//jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzU2NTEwOTIwMzAyOTE5NjgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDg4Mjk0MTIsImRldiI6IkFQUCIsImp0aSI6InEyTEtYTG1uYXJ5MGhlM0FmVE9ZcFE9PSIsImV4dCI6IiJ9.T5o3LYyncHp2H7yWhj1S+MoWCj68KrcjfpZzqf0qxL8="
		//jwtSecret := "liEnMu77ysCaU4BHy*kT^j#lKWM1JqTPhjSHNtEl#lK!ZC@diQRl04GSsDVIQnU="
		//jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzE5MjYwOTAwMTIyMTY1MjQ4IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDk3ODE5NTQsImRldiI6IiIsImp0aSI6IlZIY0o0dVlLNUtPMnRTMkdnMktkc0E9PSIsImV4dCI6IiJ9.23r2TXlxehMpDjLFcF0ywExweHqQVpxsaEqE9qj6ywM="
		//jwtSecret := "9R930u8MVhxjy5+Hy*kT^j#lKECJUx8e4U3Jg309#lK!ZC@diQYB/owhMijldsg="
		return jwtToken, jwtSecret, nil
	}

	receiveCall := func(message []byte) (interface{}, error) {
		//reply := MsgReply{}
		//if err := utils.JsonUnmarshal(message, &reply); err != nil {
		//	return nil, err
		//}
		//fmt.Println("receive:", reply)
		//reply.Ack = true
		//return &reply, nil
		fmt.Println("receive: ", string(message))
		return nil, nil
	}

	client := node.WsClient{
		//Addr:        "wss://uapi.3w.com:443",
		//Addr:        "ws://18.166.239.217:6060",
		//Path:        "/websocket",
		Addr:        "ws://localhost:6060",
		Path:        "/websocket",
		Origin:      "*",
		AuthCall:    authCall,
		ReceiveCall: receiveCall,
	}
	//go func() {
	//	for {
	//		a := map[string]interface{}{"type": "test", "aaaaaaa": utils.NextSID()}
	//		client.SendMessage(&a)
	//		time.Sleep(5 * time.Second)
	//	}
	//}()
	client.StartWebsocket(true)
}

func TestWsClient(t *testing.T) {
	for i := 0; i < 500; i++ {
		go newClient()
	}
	select {}
}

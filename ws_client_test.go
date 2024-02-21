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
		//jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzU2NTEwOTIwMzAyOTE5NjgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDg4Mjk0MTIsImRldiI6IkFQUCIsImp0aSI6InEyTEtYTG1uYXJ5MGhlM0FmVE9ZcFE9PSIsImV4dCI6IiJ9.T5o3LYyncHp2H7yWhj1S+MoWCj68KrcjfpZzqf0qxL8="
		//jwtSecret := "liEnMu77ysCaU4BHy*kT^j#lKWM1JqTPhjSHNtEl#lK!ZC@diQRl04GSsDVIQnU="
		return jwtToken, jwtSecret, nil
	}

	receiveCall := func(message []byte) (interface{}, error) {
		reply := MsgReply{}
		if err := utils.JsonUnmarshal(message, &reply); err != nil {
			return nil, err
		}
		fmt.Println("receive:", reply)
		reply.Ack = true
		return &reply, nil
	}

	client := node.WsClient{
		Addr:        "ws://localhost:8080",
		Path:        "/query",
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
	for i := 0; i < 10000; i++ {
		go newClient()
	}
	select {}
}

package main

import (
	"context"
	"github.com/godaddy-x/freego/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"log"
	"testing"
)

func TestMGPipe(t *testing.T) {
	opts := options.Client().ApplyURI("mongodb://localhost:27017")
	// 连接数据库
	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		log.Fatal(err)
	}
	// 判断服务是不是可用
	if err := client.Ping(context.Background(), readpref.Primary()); err != nil {
		log.Fatal(err)
	}
	for i := 0; i < 1; i++ {
		// 获取数据库和集合
		blockDB := client.Database("openserver").Collection("ow_block")
		query := make(map[string]interface{}, 0)
		op := &options.FindOptions{}
		op.SetSort(bson.M{"ctime": 1})
		op.SetLimit(5)
		op.SetProjection(bson.M{"_id": 1, "hash": 1})
		cur, err := blockDB.Find(context.Background(), query, op)
		if err != nil {
			log.Fatal(err)
		}
		var result []*OwBlock
		if err := cur.All(context.Background(), &result); err != nil {
			panic(err)
		}
		//fmt.Println(result)
	}
	walletDB := client.Database("openserver").Collection("ow_wallet")
	ctx := context.Background()
	client.UseSession(ctx, func(sessionContext mongo.SessionContext) error {
		if err := sessionContext.StartTransaction(); err != nil {
			panic(err)
		}
		wallet := &OwWallet{
			Id:       utils.NextIID(),
			WalletID: "我要test",
		}
		_, err := walletDB.InsertOne(sessionContext, &wallet)
		if err != nil {
			panic(err)
		}
		_, err = walletDB.InsertOne(sessionContext, &wallet)
		if err != nil {
			panic(err)
		}
		sessionContext.AbortTransaction(sessionContext)
		return nil
	})
}

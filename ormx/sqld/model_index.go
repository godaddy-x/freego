package sqld

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func readyCollection(object sqlc.Object) {
	db, err := NewMongo(Option{Timeout: 120000})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.Save(object); err != nil {
		panic(err)
	}
	if err := db.Delete(object); err != nil {
		panic(err)
	}
}

func dropIndex(object sqlc.Object, index []sqlc.Index) bool {
	readyCollection(object)
	db, err := NewMongo(Option{Timeout: 120000})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	coll, err := db.GetDatabase(object.GetTable())
	if err != nil {
		panic(err)
	}
	cur, err := coll.Indexes().List(context.Background())
	if err != nil {
		panic(err)
	}
	var list []map[string]interface{}
	if err := cur.All(context.Background(), &list); err != nil {
		panic(err)
	}
	oldKey := ""
	for _, v := range list {
		key := v["name"].(string)
		if key == "_id_" {
			continue
		}
		oldKey += key
	}
	newKey := ""
	for _, v := range index {
		newKey += v.Name
	}
	if oldKey == newKey {
		return false
	}
	if _, err := coll.Indexes().DropAll(context.Background()); err != nil {
		panic(err)
	}
	return true
}

func addIndex(object sqlc.Object, index sqlc.Index) error {
	db, err := NewMongo(Option{Timeout: 120000})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	coll, err := db.GetDatabase(object.GetTable())
	if err != nil {
		panic(err)
	}
	bsonD := bson.D{}
	for _, v := range index.Key {
		bsonD = append(bsonD, bson.E{Key: v, Value: 1})
	}
	modelIndex := mongo.IndexModel{
		Keys: bsonD, Options: &options.IndexOptions{Name: &index.Name, Unique: &index.Unique},
	}
	if _, err := coll.Indexes().CreateOne(context.Background(), modelIndex); err != nil {
		panic(err)
	}
	return nil
}

// RebuildMongoDBIndex 先删除所有表索引,再按配置新建(线上慎用功能)
func RebuildMongoDBIndex() error {
	for _, model := range modelDrivers {
		index := model.Object.NewIndex()
		if index == nil {
			continue
		}
		if !dropIndex(model.Object, index) {
			fmt.Println(fmt.Sprintf("********* [%s] index consistent, skipping *********", model.Object.GetTable()))
			continue
		}
		fmt.Println(fmt.Sprintf("********* [%s] delete all index *********", model.Object.GetTable()))
		for _, v := range index {
			addIndex(model.Object, v)
			fmt.Println(fmt.Sprintf("********* [%s] add index [%s] *********", model.Object.GetTable(), v.Name))
		}
	}
	return nil
}

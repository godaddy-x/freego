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
	db, err := NewMongo()
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

func dropIndex(object sqlc.Object) error {
	readyCollection(object)
	db, err := NewMongo()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	coll, err := db.GetDatabase(object.GetTable())
	if err != nil {
		panic(err)
	}
	if _, err := coll.Indexes().DropAll(context.Background()); err != nil {
		panic(err)
	}
	return nil
}

func addIndex(object sqlc.Object, index sqlc.Index) error {
	db, err := NewMongo()
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
		dropIndex(model.Object)
		fmt.Println(fmt.Sprintf("********* [%s] delete all index *********", model.Object.GetTable()))
		for _, v := range index {
			addIndex(model.Object, v)
			fmt.Println(fmt.Sprintf("********* [%s] add index [%s] *********", model.Object.GetTable(), v.Name))
		}
	}
	return nil
}

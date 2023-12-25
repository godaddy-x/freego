package sqld

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"reflect"
	"sort"
)

type IndexInfo struct {
	Table        string         // 索引所属的表名
	NonUnique    int            // 索引是否是唯一索引。如果值为 0，则表示索引是唯一索引；如果值为 1，则表示索引是普通索引
	KeyName      string         // 索引的名称
	SeqInIndex   int            // 索引中的列的顺序号
	ColumnName   string         // 索引的列名
	Collation    string         // 索引列的排序规则
	Cardinality  interface{}    // 索引列的基数，即不重复的索引值数量
	SubPart      sql.NullString // 索引的子部分长度。通常用于前缀索引，以指示索引的前缀长度
	Packed       sql.NullString // 索引存储的方式
	Null         interface{}    // 索引列是否可以包含 NULL 值
	IndexType    string         // 索引的类型，如 BTREE、HASH 等
	Comment      string         // 索引的注释信息
	IndexComment string         // 索引的额外注释信息
}

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

func dropMongoIndex(object sqlc.Object, index []sqlc.Index) bool {
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

func dropMysqlIndex(object sqlc.Object, index []sqlc.Index) bool {
	db, err := NewMysql(Option{Timeout: 120000})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	// 执行查询获取索引信息
	rows, err := db.Db.Query("SHOW INDEX FROM " + object.GetTable())
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var index IndexInfo
		if err := rows.Scan(
			&index.Table, &index.NonUnique, &index.KeyName, &index.SeqInIndex,
			&index.ColumnName, &index.Collation, &index.Cardinality, &index.SubPart,
			&index.Packed, &index.Null,
			&index.IndexType, &index.Comment, &index.IndexComment,
		); err != nil {
			panic(err)
		}
		indexes = append(indexes, index)
	}

	check := map[string][]string{}
	for _, v := range indexes {
		key := v.KeyName
		if key == "PRIMARY" {
			continue
		}
		m, b := check[key]
		if b {
			check[v.KeyName] = append(m, v.ColumnName)
		} else {
			check[v.KeyName] = []string{v.ColumnName}
		}
	}
	var drop bool
	for _, v := range index {
		if len(v.Name) == 0 || len(v.Key) == 0 {
			panic("table index name/key invalid: " + object.GetTable())
		}
		key, b := check[v.Name]
		if b {
			sort.Strings(key)
			sort.Strings(v.Key)
			if reflect.DeepEqual(key, v.Key) {
				continue
			}
		}
		drop = true
		break
	}
	if !drop {
		return false
	}
	for k, _ := range check { // 确定删除表所有索引
		if _, err := db.Db.Exec("DROP INDEX " + k + " ON " + object.GetTable()); err != nil {
			panic(err)
		}
	}
	return true
}

func addMongoIndex(object sqlc.Object, index sqlc.Index) error {
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

func addMysqlIndex(object sqlc.Object, index sqlc.Index) error {
	if len(index.Key) == 0 {
		zlog.Warn("addMysqlIndex keys is nil", 0, zlog.Any("object", object))
		return nil
	}
	if len(index.Name) == 0 {
		panic("index key name is nil: " + object.GetTable())
	}
	var columns string
	for _, v := range index.Key {
		if len(v) == 0 {
			panic("index key field is nil: " + object.GetTable())
		}
		columns += utils.AddStr(",", v)
	}
	sql := "CREATE"
	if index.Unique {
		sql = utils.AddStr(sql, " UNIQUE ")
	}
	sql = utils.AddStr(sql, " INDEX ")
	sql = utils.AddStr(sql, index.Name)
	sql = utils.AddStr(sql, " ON ", object.GetTable(), " (")
	sql = utils.AddStr(sql, columns[1:], ")")

	db, err := NewMysql(Option{Timeout: 120000})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if _, err := db.Db.Exec(sql); err != nil {
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
		if !dropMongoIndex(model.Object, index) {
			fmt.Println(fmt.Sprintf("********* [%s] index consistent, skipping *********", model.Object.GetTable()))
			continue
		}
		fmt.Println(fmt.Sprintf("********* [%s] delete all index *********", model.Object.GetTable()))
		for _, v := range index {
			addMongoIndex(model.Object, v)
			fmt.Println(fmt.Sprintf("********* [%s] add index [%s] *********", model.Object.GetTable(), v.Name))
		}
	}
	return nil
}

// RebuildMysqlDBIndex 先删除所有表索引,再按配置新建(线上慎用功能)
func RebuildMysqlDBIndex() error {
	for _, model := range modelDrivers {
		index := model.Object.NewIndex()
		//if index == nil {
		//	continue
		//}
		if !dropMysqlIndex(model.Object, index) {
			fmt.Println(fmt.Sprintf("********* [%s] index consistent, skipping *********", model.Object.GetTable()))
			continue
		}
		fmt.Println(fmt.Sprintf("********* [%s] delete all index *********", model.Object.GetTable()))
		for _, v := range index {
			addMysqlIndex(model.Object, v)
			fmt.Println(fmt.Sprintf("********* [%s] add index [%s] *********", model.Object.GetTable(), v.Name))
		}
	}
	return nil
}

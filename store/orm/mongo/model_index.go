package mongo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	sqlc "github.com/godaddy-x/freego/core/query"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	collection, err := db.Session.Database(db.Database).ListCollectionNames(ctx, bson.M{"name": object.GetTable()})
	if err != nil {
		panic(err)
	}
	if len(collection) == 0 {
		if err := db.Session.Database(db.Database).CreateCollection(ctx, object.GetTable()); err != nil {
			panic(err)
		}
	}
}

func mongoIndexDirection(dir interface{}) int {
	switch d := dir.(type) {
	case int32:
		return int(d)
	case int64:
		return int(d)
	case int:
		return d
	case float64:
		return int(d)
	default:
		return 1
	}
}

func normalizedIndexKeys(keys []sqlc.KV) string {
	var b strings.Builder
	for _, kv := range keys {
		val := kv.V
		if val != -1 {
			val = 1
		}
		b.WriteString(kv.K)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(val))
		b.WriteByte(';')
	}
	return b.String()
}

func mongoKeyDefinitionFromD(key bson.D) string {
	if len(key) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(key))
	for _, e := range key {
		pairs = append(pairs, fmt.Sprintf("%s:%d", e.Key, mongoIndexDirection(e.Value)))
	}
	return strings.Join(pairs, ";") + ";"
}

func mongoIndexDefinition(index sqlc.Index) string {
	var b strings.Builder
	b.WriteString(index.Name)
	b.WriteByte('|')
	if index.Unique {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	if index.Sparse {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	b.WriteString(normalizedIndexKeys(index.Keys))
	return b.String()
}

func mongoSpecFromIndexSpecification(spec mongo.IndexSpecification) string {
	var b strings.Builder
	b.WriteString(spec.Name)
	b.WriteByte('|')
	unique := spec.Unique != nil && *spec.Unique
	if unique {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	sparse := spec.Sparse != nil && *spec.Sparse
	if sparse {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	var keyDoc bson.D
	if len(spec.KeysDocument) > 0 {
		_ = bson.Unmarshal(spec.KeysDocument, &keyDoc)
	}
	b.WriteString(mongoKeyDefinitionFromD(keyDoc))
	return b.String()
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
	specs, err := coll.Indexes().ListSpecifications(context.Background())
	if err != nil {
		panic(err)
	}
	desired := make(map[string]string, len(index))
	for _, v := range index {
		desired[v.Name] = mongoIndexDefinition(v)
	}
	existing := make(map[string]string)
	for _, spec := range specs {
		if spec.Name == "_id_" {
			continue
		}
		existing[spec.Name] = mongoSpecFromIndexSpecification(*spec)
	}
	if len(existing) != len(desired) {
		if _, err := coll.Indexes().DropAll(context.Background()); err != nil {
			panic(err)
		}
		return true
	}
	for name, spec := range desired {
		if existing[name] != spec {
			if _, err := coll.Indexes().DropAll(context.Background()); err != nil {
				panic(err)
			}
			return true
		}
	}
	return false
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
	for _, v := range index.Keys {
		val := v.V
		if val != -1 { // 如果非倒序则默认正序
			val = 1
		}
		bsonD = append(bsonD, bson.E{Key: v.K, Value: val})
	}
	opts := &options.IndexOptions{Name: &index.Name, Unique: &index.Unique}
	if index.Sparse {
		sparse := true
		opts.Sparse = &sparse
	}
	modelIndex := mongo.IndexModel{
		Keys: bsonD, Options: opts,
	}
	if _, err := coll.Indexes().CreateOne(context.Background(), modelIndex); err != nil {
		panic(err)
	}
	return nil
}

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

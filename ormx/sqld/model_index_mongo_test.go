package sqld

import (
	"testing"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMongoIndexDefinitionMatchesSpecification(t *testing.T) {
	idx := sqlc.Index{
		Name:   "uniq_sid_reqType",
		Unique: true,
		Keys: []sqlc.KV{
			{K: "sid", V: 1},
			{K: "reqType", V: 1},
		},
	}
	keyDoc, err := bson.Marshal(bson.D{{Key: "sid", Value: 1}, {Key: "reqType", Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	unique := true
	spec := mongo.IndexSpecification{
		Name:         "uniq_sid_reqType",
		KeysDocument: keyDoc,
		Unique:       &unique,
	}
	if got, want := mongoIndexDefinition(idx), mongoSpecFromIndexSpecification(spec); got != want {
		t.Fatalf("spec mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestMongoIndexDefinitionSparse(t *testing.T) {
	idx := sqlc.Index{
		Name:   "uniq_pendingTradeSign",
		Unique: true,
		Sparse: true,
		Keys:   []sqlc.KV{{K: "pendingTradeSign", V: 1}},
	}
	keyDoc, err := bson.Marshal(bson.D{{Key: "pendingTradeSign", Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	unique, sparse := true, true
	spec := mongo.IndexSpecification{
		Name:         "uniq_pendingTradeSign",
		KeysDocument: keyDoc,
		Unique:       &unique,
		Sparse:       &sparse,
	}
	if got, want := mongoIndexDefinition(idx), mongoSpecFromIndexSpecification(spec); got != want {
		t.Fatalf("sparse spec mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestMongoIndexDefinitionRoundTripCases(t *testing.T) {
	cases := []struct {
		name string
		idx  sqlc.Index
		key  bson.D
		uniq bool
		spr  bool
	}{
		{
			name: "ow_trade createAt desc",
			idx:  sqlc.Index{Name: "createAt", Keys: []sqlc.KV{{K: "createAt", V: -1}}},
			key:  bson.D{{Key: "createAt", Value: int32(-1)}},
		},
		{
			name: "ow_trade symbol_txID non-unique",
			idx:  sqlc.Index{Name: "symbol_txID", Keys: []sqlc.KV{{K: "symbol", V: 1}, {K: "txID", V: 1}}},
			key:  bson.D{{Key: "symbol", Value: int32(1)}, {Key: "txID", Value: int32(1)}},
		},
		{
			name: "compound desc tail",
			idx:  sqlc.Index{Name: "sid_createAt", Keys: []sqlc.KV{{K: "sid", V: 1}, {K: "createAt", V: -1}}},
			key:  bson.D{{Key: "sid", Value: int32(1)}, {Key: "createAt", Value: int32(-1)}},
		},
		{
			name: "balance_log _id tail",
			idx:  sqlc.Index{Name: "accountID", Keys: []sqlc.KV{{K: "accountID", V: 1}, {K: "_id", V: -1}}},
			key:  bson.D{{Key: "accountID", Value: int32(1)}, {Key: "_id", Value: int32(-1)}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keyDoc, err := bson.Marshal(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			var uniqPtr, sprPtr *bool
			if tc.uniq {
				v := true
				uniqPtr = &v
			}
			if tc.spr {
				v := true
				sprPtr = &v
			}
			spec := mongo.IndexSpecification{
				Name:         tc.idx.Name,
				KeysDocument: keyDoc,
				Unique:       uniqPtr,
				Sparse:       sprPtr,
			}
			if got, want := mongoIndexDefinition(tc.idx), mongoSpecFromIndexSpecification(spec); got != want {
				t.Fatalf("spec mismatch\ngot:  %q\nwant: %q", got, want)
			}
		})
	}
}

func TestMongoIndexDefinitionDetectsSparseChange(t *testing.T) {
	idx := sqlc.Index{
		Name:   "uniq_pendingTradeSign",
		Unique: true,
		Sparse: true,
		Keys:   []sqlc.KV{{K: "pendingTradeSign", V: 1}},
	}
	keyDoc, err := bson.Marshal(bson.D{{Key: "pendingTradeSign", Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	unique := true
	oldSpec := mongo.IndexSpecification{
		Name:         "uniq_pendingTradeSign",
		KeysDocument: keyDoc,
		Unique:       &unique,
		// Sparse nil = false, config true => must differ
	}
	if got, want := mongoIndexDefinition(idx), mongoSpecFromIndexSpecification(oldSpec); got == want {
		t.Fatalf("expected sparse migration mismatch, got same %q", got)
	}
}

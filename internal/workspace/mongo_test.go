package workspace

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strings"
	"testing"
)

func TestMongoQueryPreservesTypesAndSortOrder(t *testing.T) {
	in, filter, opts, e := parseMongoQuery(`{"collection":"orders","filter":{"_id":{"$oid":"507f1f77bcf86cd799439011"},"amount":{"$numberLong":"9007199254740993"}},"projection":{"amount":1},"sort":{"status":1,"created":-1},"limit":2,"skip":4}`)
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := filter[0].Value.(primitive.ObjectID); !ok {
		t.Fatalf("ObjectID became %T", filter[0].Value)
	}
	if filter[1].Value != int64(9007199254740993) {
		t.Fatal("numeric precision lost", filter)
	}
	ordered := opts.Sort.(bson.D)
	if ordered[0].Key != "status" || ordered[1].Key != "created" || ordered[2].Key != "_id" {
		t.Fatal(ordered)
	}
	if *opts.Limit != 3 || *opts.Skip != 4 {
		t.Fatal(opts)
	}
	next, _, nextOpts, e := parseMongoQuery(mongoPageQuery(in, 6))
	if e != nil || next.Skip != 6 || nextOpts.Sort.(bson.D)[0].Key != "status" {
		t.Fatalf("pagination loses query: %+v %v", next, e)
	}
}
func TestMongoQueryRejectsUnsupportedOrUnboundedRequests(t *testing.T) {
	for _, q := range []string{
		`{"collection":"x","filter":[]}`, `{"collection":"x","sort":{"x":"1"}}`,
		`{"collection":"x","limit":501}`, `{"collection":"x","skip":-1}`, `{"collection":"x","skip":1000001}`,
		`{"collection":"x","pipeline":[]}`, `{"collection":"x"} {}`, `{"collection":"x","projection":null}`,
	} {
		if _, _, _, e := parseMongoQuery(q); e == nil {
			t.Errorf("accepted %s", q)
		}
	}
	if _, _, _, e := parseMongoQuery(strings.Repeat("x", (1<<20)+1)); e == nil {
		t.Fatal("accepted huge query")
	}
	if mongoAuthSource(Connection{}) != "admin" || mongoAuthSource(Connection{AuthSource: "team"}) != "team" {
		t.Fatal("auth source default")
	}
}

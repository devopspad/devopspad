package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoQuery struct {
	Collection string          `json:"collection"`
	Filter     json.RawMessage `json:"filter"`
	Projection json.RawMessage `json:"projection,omitempty"`
	Sort       json.RawMessage `json:"sort,omitempty"`
	Skip       int64           `json:"skip"`
	Limit      int64           `json:"limit"`
}

func mongoAuthSource(c Connection) string {
	if c.AuthSource == "" {
		return "admin"
	}
	return c.AuthSource
}
func mongoDocument(raw json.RawMessage, label string) (bson.D, error) {
	if len(raw) == 0 {
		return bson.D{}, nil
	}
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	var d bson.D
	if err := bson.UnmarshalExtJSON(raw, false, &d); err != nil {
		return nil, fmt.Errorf("Invalid %s: %w", label, err)
	}
	return d, nil
}
func parseMongoQuery(q string) (mongoQuery, bson.D, *options.FindOptions, error) {
	var in mongoQuery
	fail := func(e error) (mongoQuery, bson.D, *options.FindOptions, error) { return in, nil, nil, e }
	if len(q) > 1<<20 {
		return fail(errors.New("Query exceeds 1 MB"))
	}
	decoder := json.NewDecoder(strings.NewReader(q))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return fail(fmt.Errorf("Invalid find query: %w", err))
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fail(errors.New("Enter one JSON find query"))
	}
	if in.Collection == "" || strings.ContainsRune(in.Collection, 0) {
		return fail(errors.New("A collection name is required"))
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Limit < 1 || in.Limit > RowLimit {
		return fail(fmt.Errorf("Limit must be between 1 and %d", RowLimit))
	}
	if in.Skip < 0 || in.Skip > 1_000_000 {
		return fail(errors.New("Skip must be between 0 and 1000000; use a range filter for deeper pagination"))
	}
	filter, e := mongoDocument(in.Filter, "filter")
	if e != nil {
		return fail(e)
	}
	projection, e := mongoDocument(in.Projection, "projection")
	if e != nil {
		return fail(e)
	}
	sort, e := mongoDocument(in.Sort, "sort")
	if e != nil {
		return fail(e)
	}
	// A unique tie-breaker keeps unchanged data stable between offset-based batches.
	hasID := false
	for _, field := range sort {
		if field.Key == "_id" {
			hasID = true
		}
		directionOK := false
		switch v := field.Value.(type) {
		case int32:
			directionOK = v == 1 || v == -1
		case int64:
			directionOK = v == 1 || v == -1
		case float64:
			directionOK = v == 1 || v == -1
		}
		if !directionOK {
			return fail(errors.New("Sort directions must be 1 or -1"))
		}
	}
	if !hasID {
		sort = append(sort, bson.E{Key: "_id", Value: 1})
	}
	// Preserve the submitted field order in generated next/previous queries.
	in.Sort, e = bson.MarshalExtJSON(sort, false, false)
	if e != nil {
		return fail(e)
	}
	if len(in.Filter) == 0 {
		in.Filter = json.RawMessage(`{}`)
	}
	opts := options.Find().SetLimit(in.Limit + 1).SetSkip(in.Skip).SetSort(sort).SetProjection(projection).SetMaxTime(15 * time.Second)
	return in, filter, opts, nil
}
func mongoPageQuery(in mongoQuery, skip int64) string {
	in.Skip = skip
	raw, _ := json.MarshalIndent(in, "", "  ")
	return string(raw)
}
func mongoFind(ctx context.Context, c Connection, q string) (Result, error) {
	result := emptyResult()
	in, filter, opts, e := parseMongoQuery(q)
	if e != nil {
		return result, e
	}
	if c.Database == "" {
		return result, errors.New("Select a database before running a find query")
	}
	cl, e := mongoClient(ctx, c)
	if e != nil {
		return result, e
	}
	defer cl.Disconnect(context.Background())
	cursor, e := cl.Database(c.Database).Collection(in.Collection).Find(ctx, filter, opts)
	if e != nil {
		return result, e
	}
	defer cursor.Close(ctx)
	result.Columns = []string{"document"}
	result.Offset = in.Skip
	size := 0
	for cursor.Next(ctx) {
		if int64(len(result.Rows)) == in.Limit {
			result.Truncated = true
			if in.Skip+in.Limit <= 1_000_000 {
				result.NextQuery = mongoPageQuery(in, in.Skip+in.Limit)
			}
			break
		}
		raw, e := bson.MarshalExtJSON(cursor.Current, true, false)
		if e != nil {
			return result, e
		}
		size += len(raw)
		if size > 8<<20 {
			return emptyResult(), errors.New("Result exceeds 8 MB; reduce the limit or use a projection")
		}
		var doc any
		if e = json.Unmarshal(raw, &doc); e != nil {
			return result, e
		}
		result.Rows = append(result.Rows, []any{doc})
	}
	if e = cursor.Err(); e != nil {
		return result, e
	}
	if in.Skip > 0 {
		result.PreviousQuery = mongoPageQuery(in, max(0, in.Skip-in.Limit))
	}
	result.Message = "Offset-based batches use an _id tie-breaker. Concurrent writes can change page contents."
	return result, nil
}

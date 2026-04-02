package mongo

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/tech-comparison-lab/loadgen-db/internal/gen"
)

// Bench drives MongoDB benchmark operations.
type Bench struct {
	client *mongo.Client
	coll   *mongo.Collection
	db     *mongo.Database
}

// New connects to MongoDB and pings the server.
func New(ctx context.Context, dsn string) (*Bench, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(dsn))
	if err != nil {
		return nil, fmt.Errorf("mongo.Connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("ping: %w", err)
	}
	db := client.Database("bench")
	return &Bench{client: client, coll: db.Collection("orders"), db: db}, nil
}

// Close disconnects the client.
func (b *Bench) Close(ctx context.Context) {
	_ = b.client.Disconnect(ctx)
}

// Drop removes the orders collection.
func (b *Bench) Drop(ctx context.Context) error {
	return b.coll.Drop(ctx)
}

// EnsureIndexes creates the required indexes and returns the build duration.
func (b *Bench) EnsureIndexes(ctx context.Context) (time.Duration, error) {
	models := []mongo.IndexModel{
		{Keys: bson.D{{Key: "user.country", Value: 1}}},
		{Keys: bson.D{{Key: "user.id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
	}
	start := time.Now()
	_, err := b.coll.Indexes().CreateMany(ctx, models)
	return time.Since(start), err
}

// Insert inserts count documents in batches of batchSize using workers goroutines.
func (b *Bench) Insert(ctx context.Context, count, batchSize, workers int) ([]time.Duration, time.Duration, error) {
	totalBatches := count / batchSize
	if count%batchSize != 0 {
		totalBatches++
	}

	type job struct {
		docs []gen.Order
	}
	jobs := make(chan job, workers*2)
	durs := make([]time.Duration, 0, totalBatches)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	insertOpts := options.InsertMany().SetOrdered(false)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				docs := make([]interface{}, len(j.docs))
				for k, d := range j.docs {
					docs[k] = d
				}
				start := time.Now()
				if _, err := b.coll.InsertMany(ctx, docs, insertOpts); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				durs = append(durs, time.Since(start))
				mu.Unlock()
			}
		}()
	}

	start := time.Now()
	remaining := count
	for remaining > 0 {
		size := batchSize
		if remaining < batchSize {
			size = remaining
		}
		jobs <- job{docs: gen.Batch(size)}
		remaining -= size
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	return durs, total, firstErr
}

// Query runs iterations of: find({user.country: 'US'}).limit(100)
// All documents are decoded to include deserialization cost — comparable to Postgres row scanning.
func (b *Bench) Query(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	filter := bson.D{{Key: "user.country", Value: "US"}}
	opts := options.Find().SetLimit(100)
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		cur, err := b.coll.Find(ctx, filter, opts)
		if err != nil {
			return durs, time.Since(start), err
		}
		// Drain cursor to include network + BSON deserialization cost.
		var results []bson.M
		if err := cur.All(ctx, &results); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// Agg runs iterations of: $group by user.id, $sum quantity, $sort desc, $limit 100
func (b *Bench) Agg(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$user.id"},
			{Key: "total_qty", Value: bson.D{{Key: "$sum", Value: "$quantity"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total_qty", Value: -1}}}},
		{{Key: "$limit", Value: 100}},
	}
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		cur, err := b.coll.Aggregate(ctx, pipeline)
		if err != nil {
			return durs, time.Since(start), err
		}
		if err := cur.Close(ctx); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// Update runs iterations of: find 1000 US docs, set metadata.session = new UUID
func (b *Bench) Update(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	findOpts := options.Find().
		SetLimit(1000).
		SetProjection(bson.D{{Key: "_id", Value: 1}})

	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()

		// Find 1000 matching doc IDs
		cur, err := b.coll.Find(ctx, bson.D{{Key: "user.country", Value: "US"}}, findOpts)
		if err != nil {
			return durs, time.Since(start), err
		}
		var docs []struct {
			ID string `bson:"_id"`
		}
		if err := cur.All(ctx, &docs); err != nil {
			return durs, time.Since(start), err
		}
		if len(docs) == 0 {
			durs = append(durs, time.Since(t))
			continue
		}

		ids := make([]string, len(docs))
		for k, d := range docs {
			ids[k] = d.ID
		}

		// Update metadata.session for those IDs
		filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}}
		update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata.session", Value: newSessionID()}}}}
		if _, err := b.coll.UpdateMany(ctx, filter, update); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// StorageSize returns collection storageSize in bytes from collStats.
func (b *Bench) StorageSize(ctx context.Context) (int64, error) {
	cmd := bson.D{{Key: "collStats", Value: "orders"}}
	result := b.db.RunCommand(ctx, cmd)
	if result.Err() != nil {
		return 0, result.Err()
	}
	var stats struct {
		StorageSize int64 `bson:"storageSize"`
	}
	if err := result.Decode(&stats); err != nil {
		return 0, err
	}
	return stats.StorageSize, nil
}

func newSessionID() string {
	// Simple random hex string using bson ObjectID bytes
	oid := bson.NewObjectID()
	return fmt.Sprintf("%x", oid[:])
}

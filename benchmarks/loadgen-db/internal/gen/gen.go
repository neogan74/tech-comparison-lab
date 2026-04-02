package gen

import (
	"crypto/rand"
	"fmt"
	mrand "math/rand"
	"time"
)

// Order is the benchmark document schema (~500 bytes as JSON).
// Both json and bson tags are set so the struct works for PostgreSQL (JSONB)
// and MongoDB (BSON) without conversion.
type Order struct {
	ID        string   `json:"id"         bson:"_id"`
	User      UserInfo `json:"user"       bson:"user"`
	Product   ProdInfo `json:"product"    bson:"product"`
	Quantity  int      `json:"quantity"   bson:"quantity"`
	Status    string   `json:"status"     bson:"status"`
	Tags      []string `json:"tags"       bson:"tags"`
	Metadata  MetaInfo `json:"metadata"   bson:"metadata"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

type UserInfo struct {
	ID      string `json:"id"      bson:"id"`
	Country string `json:"country" bson:"country"`
	Tier    string `json:"tier"    bson:"tier"`
}

type ProdInfo struct {
	ID       string  `json:"id"       bson:"id"`
	Category string  `json:"category" bson:"category"`
	Price    float64 `json:"price"    bson:"price"`
}

type MetaInfo struct {
	Source  string `json:"source"  bson:"source"`
	Session string `json:"session" bson:"session"`
}

var (
	// ~40% US to ensure query hits
	countries = []string{"US", "US", "US", "US", "GB", "DE", "FR", "CA", "AU", "JP"}
	tiers     = []string{"free", "basic", "premium"}
	statuses  = []string{"pending", "processing", "shipped", "shipped", "delivered"}
	categories = []string{"electronics", "clothing", "books", "sports", "home"}
	sources   = []string{"web", "mobile", "api"}
	tagPool   = []string{"flash-sale", "gift", "new", "trending", "clearance",
		"bestseller", "limited", "eco", "premium", "bundle"}
)

// New generates a single Order document with random field values.
func New() Order {
	now := time.Now().UTC()
	tags := make([]string, mrand.Intn(4)) // 0-3 tags
	for i := range tags {
		tags[i] = tagPool[mrand.Intn(len(tagPool))]
	}
	return Order{
		ID: newUUID(),
		User: UserInfo{
			ID:      newUUID(),
			Country: countries[mrand.Intn(len(countries))],
			Tier:    tiers[mrand.Intn(len(tiers))],
		},
		Product: ProdInfo{
			ID:       newUUID(),
			Category: categories[mrand.Intn(len(categories))],
			Price:    float64(mrand.Intn(99900)+100) / 100.0, // 1.00 – 999.99
		},
		Quantity:  mrand.Intn(9) + 1, // 1-10
		Status:    statuses[mrand.Intn(len(statuses))],
		Tags:      tags,
		Metadata: MetaInfo{
			Source:  sources[mrand.Intn(len(sources))],
			Session: newUUID(),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Batch generates n Order documents.
func Batch(n int) []Order {
	docs := make([]Order, n)
	for i := range docs {
		docs[i] = New()
	}
	return docs
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

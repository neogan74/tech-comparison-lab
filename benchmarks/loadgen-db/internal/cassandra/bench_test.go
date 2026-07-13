package cassandra

import "testing"

func TestBucketIsStableAndBounded(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"
	want := bucket(id)
	for i := 0; i < 10; i++ {
		got := bucket(id)
		if got != want {
			t.Fatalf("bucket changed: got %d, want %d", got, want)
		}
		if got < 0 || got >= bucketCount {
			t.Fatalf("bucket out of range: %d", got)
		}
	}
}

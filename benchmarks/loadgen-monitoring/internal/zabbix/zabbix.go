package zabbix

import (
	"context"
	"fmt"
	mrand "math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/tech-comparison-lab/loadgen-monitoring/internal/bench"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/gen"
)

const (
	hostGroup = "Bench"
	hostName  = "bench-host"
	itemChunk = 500 // items created per item.create call
	// historyItems bounds the number of items pulled by the history query so
	// full runs don't drag back millions of rows in one call (see README
	// Fairness Notes).
	historyItems = 100
	// historyWindowSec is the look-back window for the history read op.
	historyWindowSec = 1800
)

// Backend drives ingest and query operations against a Zabbix server, using
// the JSON-RPC API for provisioning/reads and the sender protocol for ingest.
type Backend struct {
	api    *API
	sender *Sender
	user   string
	pass   string

	hostID  string
	itemIDs []string // series index -> itemid, filled by Provision
}

// New creates a Zabbix Backend. apiURL is the api_jsonrpc.php endpoint,
// senderAddr is the trapper "host:port", and user/password authenticate the
// API.
func New(apiURL, senderAddr, user, password string) *Backend {
	return &Backend{
		api:    NewAPI(apiURL),
		sender: NewSender(senderAddr),
		user:   user,
		pass:   password,
	}
}

// Name identifies this backend in result rows.
func (b *Backend) Name() string { return "zabbix" }

// Ping checks API reachability via apiinfo.version (no auth required).
func (b *Backend) Ping(ctx context.Context) error {
	_, err := b.api.Version(ctx)
	return err
}

func itemKey(i int) string  { return fmt.Sprintf("bench.item.%d", i) }
func itemName(i int) string { return fmt.Sprintf("series %d region=%s", i, gen.RegionOf(i)) }

// Provision logs in, ensures the bench host group and host exist, creates
// seriesN trapper items, and waits until the server's config cache has picked
// them up (so the trapper will accept values).
func (b *Backend) Provision(ctx context.Context, seriesN int) error {
	if err := b.loginWithRetry(ctx); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	groupID, err := b.api.EnsureHostGroup(ctx, hostGroup)
	if err != nil {
		return fmt.Errorf("host group: %w", err)
	}
	hostID, err := b.api.EnsureHost(ctx, hostName, groupID)
	if err != nil {
		return fmt.Errorf("host: %w", err)
	}
	b.hostID = hostID

	b.itemIDs = make([]string, 0, seriesN)
	for start := 0; start < seriesN; start += itemChunk {
		end := min(start+itemChunk, seriesN)
		specs := make([]itemSpec, 0, end-start)
		for i := start; i < end; i++ {
			specs = append(specs, itemSpec{Name: itemName(i), Key: itemKey(i)})
		}
		ids, err := b.api.CreateItems(ctx, hostID, specs)
		if err != nil {
			return fmt.Errorf("create items [%d,%d): %w", start, end, err)
		}
		b.itemIDs = append(b.itemIDs, ids...)
	}

	if err := b.waitItemsActive(ctx); err != nil {
		return fmt.Errorf("waiting for items to become active: %w", err)
	}
	return nil
}

// loginWithRetry authenticates, retrying for up to two minutes to ride out the
// one-time DB schema import the Zabbix server performs on a fresh volume
// (until it completes, the frontend can't authenticate against the users table).
func (b *Backend) loginWithRetry(ctx context.Context) error {
	deadline := time.Now().Add(2 * time.Minute)
	var lastErr error
	for {
		if err := b.api.Login(ctx, b.user, b.pass); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// waitItemsActive polls the trapper with a probe value for item 0 until the
// server accepts it (processed >= 1), tolerating the config-cache refresh
// delay after item.create.
func (b *Backend) waitItemsActive(ctx context.Context) error {
	probe := []senderValue{{
		Host:  hostName,
		Key:   itemKey(0),
		Value: "0",
		Clock: time.Now().Unix(),
	}}
	deadline := time.Now().Add(2 * time.Minute)
	var lastInfo string
	for time.Now().Before(deadline) {
		resp, err := b.sender.send(ctx, probe)
		if err == nil && parseProcessed(resp.Info) >= 1 {
			return nil
		}
		if err != nil {
			lastInfo = err.Error()
		} else {
			lastInfo = resp.Info
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("items not active before deadline (last: %s)", lastInfo)
}

// Ingest sends count samples across seriesN items via the sender protocol,
// batching batchSize samples per request across workers connections.
func (b *Backend) Ingest(ctx context.Context, count, seriesN, batchSize, workers, intervalSec int) ([]time.Duration, time.Duration, error) {
	samplesPerSeries := max(1, count/seriesN)
	seriesPerBatch := max(1, batchSize/samplesPerSeries)
	intervalSecI := int64(intervalSec)
	nowSec := time.Now().Unix()
	startSec := nowSec - int64(samplesPerSeries)*intervalSecI

	type jobT struct{ start, end int }
	jobs := make(chan jobT, workers*2)

	var mu sync.Mutex
	durs := make([]time.Duration, 0, seriesN/seriesPerBatch+1)
	var firstErr error
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rnd := mrand.New(mrand.NewSource(seed))
			for j := range jobs {
				values := make([]senderValue, 0, (j.end-j.start)*samplesPerSeries)
				for i := j.start; i < j.end; i++ {
					key := itemKey(i)
					for t := 0; t < samplesPerSeries; t++ {
						values = append(values, senderValue{
							Host:  hostName,
							Key:   key,
							Value: strconv.FormatFloat(gen.Value(i, t, rnd), 'f', 4, 64),
							Clock: startSec + int64(t)*intervalSecI,
						})
					}
				}

				t0 := time.Now()
				resp, err := b.sender.send(ctx, values)
				if err == nil && parseProcessed(resp.Info) == 0 {
					err = fmt.Errorf("trapper processed 0 values (info: %s)", resp.Info)
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				durs = append(durs, time.Since(t0))
				mu.Unlock()
			}
		}(int64(w + 1))
	}

	start := time.Now()
	for i := 0; i < seriesN; i += seriesPerBatch {
		end := min(i+seriesPerBatch, seriesN)
		jobs <- jobT{start: i, end: end}
	}
	close(jobs)
	wg.Wait()
	return durs, time.Since(start), firstErr
}

// Query maps a bench.Op* to a Zabbix API read and times iters calls.
func (b *Backend) Query(ctx context.Context, op string, iters int) ([]time.Duration, time.Duration, error) {
	if b.hostID == "" {
		return nil, 0, fmt.Errorf("backend not provisioned")
	}

	var fn func(context.Context) error
	switch op {
	case bench.OpLatest:
		fn = func(ctx context.Context) error {
			_, err := b.api.LatestValues(ctx, b.hostID, "")
			return err
		}
	case bench.OpFiltered:
		fn = func(ctx context.Context) error {
			_, err := b.api.LatestValues(ctx, b.hostID, "region=us-east-1")
			return err
		}
	case bench.OpHistory:
		subset := b.itemIDs
		if len(subset) > historyItems {
			subset = subset[:historyItems]
		}
		fn = func(ctx context.Context) error {
			_, err := b.api.History(ctx, subset, historyWindowSec)
			return err
		}
	default:
		return nil, 0, fmt.Errorf("unknown op %q", op)
	}

	durs := make([]time.Duration, 0, iters)
	start := time.Now()
	for i := 0; i < iters; i++ {
		t0 := time.Now()
		if err := fn(ctx); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t0))
	}
	return durs, time.Since(start), nil
}

// StorageBytes returns 0: Zabbix stores history in an external RDBMS
// (PostgreSQL here), which exposes no single comparable on-disk figure via the
// API. See the experiment README Fairness Notes.
func (b *Backend) StorageBytes(ctx context.Context) (int64, error) { return 0, nil }

package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Client wraps a NATS connection and provides helpers for all subsystems.
type Client struct {
	nc *nats.Conn
	js jetstream.JetStream
}

// ConnectOptions holds optional connection parameters.
type ConnectOptions = nats.Option

// BuildConnectOptions constructs nats.Options from optional creds/nkey paths and user/password.
func BuildConnectOptions(credsFile, nkeyFile, user, password string) []nats.Option {
	var opts []nats.Option
	if credsFile != "" {
		opts = append(opts, nats.UserCredentials(credsFile))
	}
	if nkeyFile != "" {
		if opt, err := nats.NkeyOptionFromSeed(nkeyFile); err == nil {
			opts = append(opts, opt)
		}
	}
	if user != "" || password != "" {
		opts = append(opts, nats.UserInfo(user, password))
	}
	return opts
}

// Connect establishes a NATS connection.
func Connect(url string, opts ...nats.Option) (*Client, error) {
	defaultOpts := []nats.Option{
		nats.Name("nats-explorer"),
		nats.Timeout(5 * time.Second),
		nats.RetryOnFailedConnect(false),
	}
	opts = append(defaultOpts, opts...)

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	return &Client{nc: nc, js: js}, nil
}

func (c *Client) Close() {
	c.nc.Drain()
}

func (c *Client) ServerURL() string {
	return c.nc.ConnectedUrl()
}

func (c *Client) IsConnected() bool {
	return c.nc.IsConnected()
}

// --- KV Store ---

type KVBucketInfo struct {
	Name     string
	Keys     uint64
	Bytes    uint64
	History  int64
	TTL      time.Duration
}

func (c *Client) ListKVBuckets(ctx context.Context) ([]KVBucketInfo, error) {
	var buckets []KVBucketInfo
	iter := c.js.KeyValueStoreNames(ctx)
	for name := range iter.Name() {
		kv, err := c.js.KeyValue(ctx, name)
		if err != nil {
			continue
		}
		status, err := kv.Status(ctx)
		if err != nil {
			continue
		}
		buckets = append(buckets, KVBucketInfo{
			Name:    name,
			Keys:    status.Values(),
			Bytes:   status.Bytes(),
			History: int64(status.History()),
			TTL:     status.TTL(),
		})
	}
	if err := iter.Error(); err != nil {
		return buckets, err
	}
	return buckets, nil
}

type KVEntry struct {
	Key      string
	Value    []byte
	Revision uint64
	Created  time.Time
	Delta    uint64
	Op       string
}

func (c *Client) ListKVKeys(ctx context.Context, bucket string) ([]KVEntry, error) {
	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}
	// WatchAll without IgnoreDeletes so tombstoned keys appear alongside live ones.
	// Stop() is called immediately after reading initial values — no persistent subscription.
	watcher, err := kv.WatchAll(ctx)
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()

	var entries []KVEntry
	for entry := range watcher.Updates() {
		if entry == nil {
			// nil signals end of initial values; subscription is stopped via defer
			break
		}
		entries = append(entries, KVEntry{
			Key:      entry.Key(),
			Value:    entry.Value(),
			Revision: entry.Revision(),
			Created:  entry.Created(),
			Op:       entry.Operation().String(),
		})
	}
	return entries, nil
}

// --- Object Store ---

type ObjBucketInfo struct {
	Name        string
	Description string
	Objects     int
	Size        uint64
}

func (c *Client) ListObjBuckets(ctx context.Context) ([]ObjBucketInfo, error) {
	var buckets []ObjBucketInfo
	iter := c.js.ObjectStoreNames(ctx)
	for name := range iter.Name() {
		obs, err := c.js.ObjectStore(ctx, name)
		if err != nil {
			continue
		}
		status, err := obs.Status(ctx)
		if err != nil {
			continue
		}
		buckets = append(buckets, ObjBucketInfo{
			Name:        name,
			Description: status.Description(),
			Size:        status.Size(),
		})
	}
	if err := iter.Error(); err != nil {
		return buckets, err
	}
	return buckets, nil
}

type ObjEntry struct {
	Name        string
	Description string
	Size        uint64
	Chunks      uint32
	Digest      string
	Modified    time.Time
}

func (c *Client) ListObjEntries(ctx context.Context, bucket string) ([]ObjEntry, error) {
	obs, err := c.js.ObjectStore(ctx, bucket)
	if err != nil {
		return nil, err
	}
	list, err := obs.List(ctx)
	if err != nil {
		return nil, err
	}
	var entries []ObjEntry
	for _, info := range list {
		entries = append(entries, ObjEntry{
			Name:        info.Name,
			Description: info.Description,
			Size:        info.Size,
			Chunks:      info.Chunks,
			Digest:      info.Digest,
			Modified:    info.ModTime,
		})
	}
	return entries, nil
}

// --- Core NATS ---

type LiveMessage struct {
	Subject string
	Reply   string
	Time    time.Time
	Data    []byte
}

// Subscribe subscribes to a subject and sends messages to the returned channel.
// Cancel ctx to stop.
func (c *Client) Subscribe(ctx context.Context, subject string, ch chan<- LiveMessage) error {
	sub, err := c.nc.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case ch <- LiveMessage{
			Subject: msg.Subject,
			Reply:   msg.Reply,
			Time:    time.Now(),
			Data:    msg.Data,
		}:
		default:
		}
	})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()
	return nil
}

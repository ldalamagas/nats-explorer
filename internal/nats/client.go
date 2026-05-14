package nats

import (
	"context"
	"fmt"
	"strings"
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
	Name    string
	Keys    uint64
	Bytes   uint64
	History int64
	TTL     time.Duration
}

func (c *Client) ListKVBuckets(ctx context.Context) ([]KVBucketInfo, error) {
	var buckets []KVBucketInfo
	iter := c.js.StreamNames(ctx)
	for name := range iter.Name() {
		if !strings.HasPrefix(name, "KV_") {
			continue
		}
		bucket := strings.TrimPrefix(name, "KV_")
		kv, err := c.js.KeyValue(ctx, bucket)
		if err != nil {
			continue
		}
		status, err := kv.Status(ctx)
		if err != nil {
			continue
		}
		buckets = append(buckets, KVBucketInfo{
			Name:    bucket,
			Keys:    status.Values(),
			Bytes:   status.Bytes(),
			History: int64(status.History()),
			TTL:     status.TTL(),
		})
	}
	if err := iter.Err(); err != nil {
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
	iter := c.js.StreamNames(ctx)
	for name := range iter.Name() {
		if !strings.HasPrefix(name, "OBJ_") {
			continue
		}
		bucket := strings.TrimPrefix(name, "OBJ_")
		obs, err := c.js.ObjectStore(ctx, bucket)
		if err != nil {
			continue
		}
		status, err := obs.Status(ctx)
		if err != nil {
			continue
		}
		buckets = append(buckets, ObjBucketInfo{
			Name:        bucket,
			Description: status.Description(),
			Size:        status.Size(),
		})
	}
	if err := iter.Err(); err != nil {
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

// --- Streams ---

type StreamEntry struct {
	Name        string
	Description string
	Subjects    []string
	Storage     string
	Replicas    int
	Retention   string
	MaxMsgs     int64
	MaxBytes    int64
	MaxAge      time.Duration
	MaxMsgSize  int32
	Duplicates  time.Duration
	Created     time.Time
	Messages    uint64
	Bytes       uint64
	Consumers   int
	FirstSeq    uint64
	FirstTime   time.Time
	LastSeq     uint64
	LastTime    time.Time
	NumDeleted  int
	NumSubjects uint64
}

func (c *Client) ListStreams(ctx context.Context) ([]StreamEntry, error) {
	var entries []StreamEntry
	iter := c.js.ListStreams(ctx)
	for info := range iter.Info() {
		cfg := info.Config
		st := info.State
		replicas := cfg.Replicas
		if replicas == 0 {
			replicas = 1
		}
		entries = append(entries, StreamEntry{
			Name:        cfg.Name,
			Description: cfg.Description,
			Subjects:    cfg.Subjects,
			Storage:     cfg.Storage.String(),
			Replicas:    replicas,
			Retention:   cfg.Retention.String(),
			MaxMsgs:     cfg.MaxMsgs,
			MaxBytes:    cfg.MaxBytes,
			MaxAge:      cfg.MaxAge,
			MaxMsgSize:  cfg.MaxMsgSize,
			Duplicates:  cfg.Duplicates,
			Created:     info.Created,
			Messages:    st.Msgs,
			Bytes:       st.Bytes,
			Consumers:   st.Consumers,
			FirstSeq:    st.FirstSeq,
			FirstTime:   st.FirstTime,
			LastSeq:     st.LastSeq,
			LastTime:    st.LastTime,
			NumDeleted:  st.NumDeleted,
			NumSubjects: st.NumSubjects,
		})
	}
	if err := iter.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

// --- KV Bucket Detail ---

type KVBucketDetail struct {
	Name         string
	Keys         uint64
	Bytes        uint64
	History      int64
	TTL          time.Duration
	Storage      string
	Replicas     int
	MaxBytes     int64
	MaxValueSize int32
	StreamName   string
	MirrorName   string
	MirrorDomain string
}

func (c *Client) GetKVBucketInfo(ctx context.Context, bucket string) (*KVBucketDetail, error) {
	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}
	status, err := kv.Status(ctx)
	if err != nil {
		return nil, err
	}
	cfg := status.Config()
	replicas := cfg.Replicas
	if replicas == 0 {
		replicas = 1
	}
	detail := &KVBucketDetail{
		Name:         status.Bucket(),
		Keys:         status.Values(),
		Bytes:        status.Bytes(),
		History:      status.History(),
		TTL:          status.TTL(),
		Storage:      cfg.Storage.String(),
		Replicas:     replicas,
		MaxBytes:     cfg.MaxBytes,
		MaxValueSize: cfg.MaxValueSize,
		StreamName:   "KV_" + bucket,
	}
	if cfg.Mirror != nil {
		detail.MirrorName = strings.TrimPrefix(cfg.Mirror.Name, "KV_")
		detail.MirrorDomain = cfg.Mirror.Domain
	}
	return detail, nil
}

// --- Object Bucket Detail ---

type ObjBucketDetail struct {
	Name        string
	Description string
	Size        uint64
	TTL         time.Duration
	Storage     string
	Replicas    int
	Sealed      bool
	StreamName  string
}

func (c *Client) GetObjBucketInfo(ctx context.Context, bucket string) (*ObjBucketDetail, error) {
	obs, err := c.js.ObjectStore(ctx, bucket)
	if err != nil {
		return nil, err
	}
	status, err := obs.Status(ctx)
	if err != nil {
		return nil, err
	}
	replicas := status.Replicas()
	if replicas == 0 {
		replicas = 1
	}
	return &ObjBucketDetail{
		Name:        status.Bucket(),
		Description: status.Description(),
		Size:        status.Size(),
		TTL:         status.TTL(),
		Storage:     status.Storage().String(),
		Replicas:    replicas,
		Sealed:      status.Sealed(),
		StreamName:  "OBJ_" + bucket,
	}, nil
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

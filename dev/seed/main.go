// seed populates a NATS server with realistic test data for nats-explorer development.
//
// Usage:
//
//	go run ./dev/seed [--url nats://localhost:4222]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	url := flag.String("url", "nats://localhost:4222", "NATS server URL")
	flag.Parse()

	nc, err := nats.Connect(*url, nats.Name("nats-explorer-seed"), nats.Timeout(5*time.Second))
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer nc.Drain()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	seedKV(ctx, js)
	seedObjectStore(ctx, js)

	fmt.Println("\nDone. Run: go run . --url", *url)
}

// ── KV Store ──────────────────────────────────────────────────────────────────

func seedKV(ctx context.Context, js jetstream.JetStream) {
	fmt.Println("── KV Store ─────────────────────")

	// config bucket
	config, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "config",
		Description: "Application runtime configuration",
		History:     5,
	})
	if err != nil {
		fmt.Printf("  kv config: %v\n", err)
	} else {
		fmt.Println("  bucket: config")
		kvPut(ctx, config, "app.name", "nats-explorer")
		kvPut(ctx, config, "app.version", "0.1.0")
		kvPut(ctx, config, "app.log_level", "info")
		kvPut(ctx, config, "app.timeout_seconds", "30")
		kvPut(ctx, config, "feature.dark_mode", "true")
		kvPut(ctx, config, "feature.beta_ui", "false")
		kvPut(ctx, config, "db.host", "postgres.internal")
		kvPut(ctx, config, "db.port", "5432")
		kvPut(ctx, config, "db.max_connections", "100")
		kvPut(ctx, config, "cache.ttl_seconds", "300")

		dbJSON, _ := json.Marshal(map[string]any{
			"host":            "postgres.internal",
			"port":            5432,
			"name":            "appdb",
			"max_connections": 100,
			"idle_connections": 10,
			"ssl_mode":        "require",
		})
		kvPutBytes(ctx, config, "db.config", dbJSON)

		rateLimitJSON, _ := json.Marshal(map[string]any{
			"enabled":           true,
			"requests_per_min":  600,
			"burst":             50,
			"exclude_paths":     []string{"/health", "/metrics"},
		})
		kvPutBytes(ctx, config, "server.rate_limit", rateLimitJSON)

		alertingJSON, _ := json.Marshal(map[string]any{
			"slack_webhook": "https://hooks.slack.com/services/xxx/yyy/zzz",
			"pagerduty_key": "abc123",
			"notify_on":     []string{"error", "critical"},
			"quiet_hours":   map[string]string{"start": "22:00", "end": "08:00"},
		})
		kvPutBytes(ctx, config, "alerting.config", alertingJSON)

		fmt.Printf("  keys: 13\n")
	}

	// sessions bucket (short TTL to simulate real usage)
	sessions, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "sessions",
		Description: "Active user sessions",
		TTL:         2 * time.Hour,
		History:     1,
	})
	if err != nil {
		fmt.Printf("  kv sessions: %v\n", err)
	} else {
		fmt.Println("  bucket: sessions")
		for i := 1; i <= 8; i++ {
			token := fmt.Sprintf("tok_%x", rand.Int63())
			val, _ := json.Marshal(map[string]any{
				"user_id":    fmt.Sprintf("user-%d", i),
				"email":      fmt.Sprintf("user%d@example.com", i),
				"created_at": time.Now().Add(-time.Duration(rand.Intn(60)) * time.Minute).Format(time.RFC3339),
				"ip":         fmt.Sprintf("10.0.%d.%d", rand.Intn(255), rand.Intn(255)),
			})
			kvPutBytes(ctx, sessions, fmt.Sprintf("sess.%s", token[:12]), val)
		}
		fmt.Printf("  keys: 8\n")
	}

	// feature-flags bucket
	flags, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "feature-flags",
		Description: "Feature flag overrides per environment",
		History:     10,
	})
	if err != nil {
		fmt.Printf("  kv feature-flags: %v\n", err)
	} else {
		fmt.Println("  bucket: feature-flags")
		kvPut(ctx, flags, "prod.new-checkout", "false")
		kvPut(ctx, flags, "prod.recommendations", "true")
		kvPut(ctx, flags, "staging.new-checkout", "true")
		kvPut(ctx, flags, "staging.recommendations", "true")
		kvPut(ctx, flags, "dev.new-checkout", "true")
		kvPut(ctx, flags, "dev.recommendations", "true")
		kvPut(ctx, flags, "dev.debug-toolbar", "true")
		fmt.Printf("  keys: 7\n")
	}
}

func kvPut(ctx context.Context, kv jetstream.KeyValue, key, value string) {
	if _, err := kv.Put(ctx, key, []byte(value)); err != nil {
		fmt.Printf("  put %s: %v\n", key, err)
	}
}

func kvPutBytes(ctx context.Context, kv jetstream.KeyValue, key string, value []byte) {
	if _, err := kv.Put(ctx, key, value); err != nil {
		fmt.Printf("  put %s: %v\n", key, err)
	}
}

// ── Object Store ──────────────────────────────────────────────────────────────

func seedObjectStore(ctx context.Context, js jetstream.JetStream) {
	fmt.Println("── Object Store ─────────────────")

	assets, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      "assets",
		Description: "Static application assets and configs",
	})
	if err != nil {
		fmt.Printf("  obj assets: %v\n", err)
	} else {
		fmt.Println("  bucket: assets")
		objPut(ctx, assets, "openapi.json", "application/json", generateOpenAPISpec())
		objPut(ctx, assets, "default-config.yaml", "text/yaml", []byte(defaultConfigYAML))
		objPut(ctx, assets, "email-template.html", "text/html", []byte(emailTemplateHTML))
		objPut(ctx, assets, "logo.svg", "image/svg+xml", []byte(logoSVG))
	}

	backups, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      "backups",
		Description: "Database snapshot backups",
	})
	if err != nil {
		fmt.Printf("  obj backups: %v\n", err)
	} else {
		fmt.Println("  bucket: backups")
		for i := 1; i <= 3; i++ {
			name := fmt.Sprintf("snapshot-%s.sql.gz", time.Now().Add(-time.Duration(i)*24*time.Hour).Format("2006-01-02"))
			size := rand.Intn(500_000) + 50_000
			objPut(ctx, backups, name, "application/gzip", bytes.Repeat([]byte("x"), size))
		}
	}
}

func objPut(ctx context.Context, obs jetstream.ObjectStore, name, contentType string, data []byte) {
	_, err := obs.PutBytes(ctx, name, data)
	if err != nil {
		fmt.Printf("  put %s: %v\n", name, err)
		return
	}
	fmt.Printf("  object: %s (%d bytes)\n", name, len(data))
}

func generateOpenAPISpec() []byte {
	spec := map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":   "Example API",
			"version": "1.0.0",
		},
		"paths": map[string]any{
			"/orders": map[string]any{
				"get":  map[string]any{"summary": "List orders"},
				"post": map[string]any{"summary": "Create order"},
			},
			"/payments": map[string]any{
				"post": map[string]any{"summary": "Process payment"},
			},
		},
	}
	data, _ := json.MarshalIndent(spec, "", "  ")
	return data
}

const defaultConfigYAML = `server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

database:
  host: postgres.internal
  port: 5432
  name: appdb
  max_connections: 100
  idle_connections: 10

cache:
  host: redis.internal
  port: 6379
  ttl: 5m

nats:
  url: nats://localhost:4222
  stream_prefix: app
`

const emailTemplateHTML = `<!DOCTYPE html>
<html>
<head><title>Order Update</title></head>
<body>
  <h1>Your order has been updated</h1>
  <p>Hello {{.Name}},</p>
  <p>Order <strong>{{.OrderID}}</strong> is now <em>{{.Status}}</em>.</p>
  <p>Thank you for shopping with us.</p>
</body>
</html>
`

const logoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <circle cx="50" cy="50" r="40" fill="#7B61FF"/>
  <text x="50" y="57" text-anchor="middle" fill="white" font-size="24" font-weight="bold">NE</text>
</svg>`

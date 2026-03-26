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

		serviceRegistryJSON, _ := json.Marshal(map[string]any{
			"version": "2",
			"services": []map[string]any{
				{"name": "api-gateway", "host": "api-gw.internal", "port": 8080, "healthcheck": "/health", "tags": []string{"edge", "public"}, "weight": 100, "tls": true},
				{"name": "auth-service", "host": "auth.internal", "port": 8081, "healthcheck": "/ping", "tags": []string{"internal"}, "weight": 100, "tls": true},
				{"name": "order-service", "host": "orders.internal", "port": 8082, "healthcheck": "/health", "tags": []string{"internal", "critical"}, "weight": 100, "tls": false},
				{"name": "payment-service", "host": "payments.internal", "port": 8083, "healthcheck": "/health", "tags": []string{"internal", "critical", "pci"}, "weight": 100, "tls": true},
				{"name": "notification-service", "host": "notify.internal", "port": 8084, "healthcheck": "/ping", "tags": []string{"internal"}, "weight": 50, "tls": false},
				{"name": "inventory-service", "host": "inventory.internal", "port": 8085, "healthcheck": "/health", "tags": []string{"internal"}, "weight": 100, "tls": false},
				{"name": "search-service", "host": "search.internal", "port": 8086, "healthcheck": "/health", "tags": []string{"internal", "public"}, "weight": 100, "tls": false},
				{"name": "recommendation-service", "host": "reco.internal", "port": 8087, "healthcheck": "/ping", "tags": []string{"internal", "beta"}, "weight": 30, "tls": false},
			},
			"load_balancer": map[string]any{"algorithm": "round-robin", "sticky_sessions": false, "timeout_ms": 5000},
			"circuit_breaker": map[string]any{"enabled": true, "threshold": 0.5, "window_seconds": 30, "min_requests": 10},
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		})
		kvPutBytes(ctx, config, "services.registry", serviceRegistryJSON)
		kvPut(ctx, config, "server.tls_certificate", tlsCertPEM)
		kvPut(ctx, config, "app.changelog", appChangelog)
		kvPut(ctx, config, "infrastructure.networking.egress.firewall.rules.allow_list", `["10.0.0.0/8","172.16.0.0/12","192.168.0.0/16"]`)
		kvPut(ctx, config, "observability.tracing.opentelemetry.exporter.otlp.endpoint", "https://otel-collector.internal:4317")
		kvPut(ctx, config, "security.authentication.oauth2.providers.google.client_secret", "GOCSPX-supersecret-placeholder-value-here")
		kvPut(ctx, config, "microservices.order-service.dependencies.payment-service.circuit_breaker.threshold_percentage", "50")

		// Seed a deleted key so the UI can render tombstone entries.
		kvPut(ctx, config, "app.deprecated_flag", "legacy-value")
		if err := config.Delete(ctx, "app.deprecated_flag"); err != nil {
			fmt.Printf("  delete app.deprecated_flag: %v\n", err)
		}

		fmt.Printf("  keys: 20 (+ 1 deleted)\n")
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

		overridesJSON, _ := json.Marshal(map[string]any{
			"schema_version": 1,
			"description":    "Full per-environment flag override matrix — managed by CI pipeline",
			"environments": map[string]any{
				"prod": map[string]any{
					"new-checkout":     map[string]any{"enabled": false, "rollout_pct": 0, "owners": []string{"checkout-team"}},
					"recommendations":  map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"ml-team"}},
					"ai-search":        map[string]any{"enabled": false, "rollout_pct": 0, "owners": []string{"search-team"}},
					"express-checkout": map[string]any{"enabled": true, "rollout_pct": 50, "owners": []string{"checkout-team"}},
					"loyalty-program":  map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"growth-team"}},
					"new-returns-flow": map[string]any{"enabled": false, "rollout_pct": 0, "owners": []string{"ops-team"}},
				},
				"staging": map[string]any{
					"new-checkout":     map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"checkout-team"}},
					"recommendations":  map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"ml-team"}},
					"ai-search":        map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"search-team"}},
					"express-checkout": map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"checkout-team"}},
					"loyalty-program":  map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"growth-team"}},
					"new-returns-flow": map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"ops-team"}},
				},
				"dev": map[string]any{
					"new-checkout":     map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"checkout-team"}},
					"recommendations":  map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"ml-team"}},
					"ai-search":        map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"search-team"}},
					"express-checkout": map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"checkout-team"}},
					"loyalty-program":  map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"growth-team"}},
					"new-returns-flow": map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"ops-team"}},
					"debug-toolbar":    map[string]any{"enabled": true, "rollout_pct": 100, "owners": []string{"platform-team"}},
				},
			},
			"updated_at": time.Now().UTC().Format(time.RFC3339),
			"updated_by": "ci-bot",
		})
		kvPutBytes(ctx, flags, "overrides.matrix", overridesJSON)
		kvPut(ctx, flags, "prod.checkout.new-multi-step-address-form.gradual-rollout-cohort-a", "false")
		kvPut(ctx, flags, "prod.payments.stripe-v3-integration.3ds2-challenge-flow-enabled", "true")
		kvPut(ctx, flags, "staging.ml.personalization.homepage-ranking-model-v4.shadow-mode", "true")
		fmt.Printf("  keys: 11\n")
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

// tlsCertPEM is a self-signed certificate used as a lengthy plain-text KV value.
const tlsCertPEM = `-----BEGIN CERTIFICATE-----
MIIFazCCA1OgAwIBAgIUYpXQME9gp3TZiXgbELMYJpWqmjowDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCVVMxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yNTAxMDEwMDAwMDBaFw0yNjAx
MDEwMDAwMDBaMEUxCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggIiMA0GCSqGSIb3DQEB
AQUAA4ICDwAwggIKAoICAQDJ5oO2yQ4O7j7B1CXXd6mCCo4Y8VY7mPfxFNTfuO3z
K9E3Xu5L1W2sVMajm4F2uFpMzBEPLLGEMpqv3kOplZhQbV1X5r3Bz0SyL8Kxz1o
YpbE5yQ8B3yThzX6OJpC4kV2HLVIFpvY6lNK8rN7XqCZPlW3Vq4MNRbCw5bUvOG
F4UfP9oQqZ5TlM3H6cGj7OKQ2nFVWrWGKl0Z3D5ioJH3q1PFXlJuGnVXtN8eGmC
KaYFpZHSOi2VqM3cDxd1Nf9oZKaObPpJBqVWBVkwT5nLpObZ3F9z6K1sIPM6HNWK
S0ORGb2Df5YXH3Ui8O1mqLqVu0vHXL7Zc1A7kq8Uxnx6PzRdXtKLhSnUJFzX6lM
SrW9G2fM4YrpFMpLCqM8yN3HqZ5B0kFCj4U5lXcBVqH8TvOgf6Py0qZ7dGWKJt8
kSlV5x0aTJVfHq6Uby7Y2KNj2p7K9dFXkzMqGpM6cRL5KX9XnKVbMaD5O1GqKJW
VsIFbHc9nO2KM3UjXq4TpCBZLKqV5Z1EJl8mPrN7mF9kX2SqLjB5oN6pMGqLhb8
I5JkHlO4dRfN2L8Vy9K3c7FJvMpN5qXpBj3Zk8LmR6Y7oT2K9zQwNjV1fC8eUar
XtP0GnZKYFqLhB4IzV9mRt5qO7dS2KXnPJcLfM8iN3GwIDAQABo1MwUTAdBgNV
HQ4EFgQUz7j2K5pqFN3XLG7Q9BVZQ5yM6KswHwYDVR0jBBgwFoAUz7j2K5pqFN3X
LG7Q9BVZQ5yM6KswDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAgEA
LJvkXnwzQ1oC9YKmpLFW2nK4q5ZlPnT3X4BnJM8qLN2VxKz7cR6pFbMqO5XdYJ1
K8jN3FpVlMnS9iQ6yR5oGzXtPkB2W7LqHmN4c1XZ6rD9mO5BnK3P8cF7qJtMvL2
S9oN4dX5kM8pQrT7zL6yBnW3JcF9oK2mR5vN8qX4pLG7hM3cO1iY6dQ9nT2kBvW
mX5F8rJ4qO3pL9cN7kM2tY6bS1nX4pZdMvQ8rT5wO2yL3cN9iF7kB1mX6qP4jL8
hK2oN5dY3mZ7cT9nO4kL6pM2rX8qB5vW1iF3yN7cZ9oK4mL2pR6tQ3nX5jO8iY7
dM1cB9kF4pL2oN6qX8rT5mZ7yW3cJ9iL4kN2oM5pQ7rT3nX6bY8jW1cF9oK4mL2
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIFazCCA1OgAwIBAgIUIntermediateCAForTestingPurposes123MA0GCSqGSIb3
DQEBCwUAMEUxCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYD
VQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwHhcNMjUwMTAxMDAwMDAwWhcN
MjYwMTAxMDAwMDAwWjBFMQswCQYDVQQGEwJVUzETMBEGA1UECAwKU29tZS1TdGF0
ZTEhMB8GA1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG
9w0BAQEFAAOCAQ8AMIIBCgKCAQEA2K5pF8mN3qL7oX9rT4cB2iY6dW1nZ5kM8pQ3
rX7vO2mL9cN4iF8kB1tY6qP5jL3hK7oN2dX8mZ4cT9nO5kL3pM7rX6qB2vW4iF8
yN3cZ6oK1mL5pR4tQ8nX2jO7iY3dM6cB4kF1pL5oN3qX6rT2mZ4yW8cJ6iL1kN5
oM2pQ4rT8nX3bY7jW6cF6oK1mL5pR3tQ7nX4jO2iY8dM3cB7kF2pL4oN8qX3rT5
mZ1yW4cJ3iL6kN8oM1pQ2rT4nX7bY3jW8cF3oK6mL1pR8tQ2nX5jO4iY1dM8cB3
kF6pL1oN4qX8rT2mZ5yW1cJ8iL3kN2oM6pQ1rT5nX3bY8jW2cF8oK3mL6pR1tQ5
nX2jO8iY4dM1cB8kF3pL6oN1qX4rT8mZ2yW6cJ1iL8kN3oM4pQ6rT1nX8bY1jW3
cF1oK8mL3pR6tQ1nX8jO1iY2dM4cB1kF8pL2oN6qX1rT4mZ8yW2cJ4iL1kN6oM3
pQ8rT6nX1bY4jW1cF4oK2mL8pR2tQ6nX1jO6iY8dM2cB4kF1pL8oN2qX6rT1mZ4
-----END CERTIFICATE-----`

// appChangelog is a lengthy plain-text KV value simulating a version history.
const appChangelog = `# Changelog

## v0.9.0 — 2026-03-01
### Added
- KV store viewer with full revision history and tombstone display
- Object store browser with size, content-type, and download metadata
- Breadcrumb navigation for all panes
- UTC timestamp normalization across all views

### Fixed
- Panic when KV bucket has zero entries
- Object store listing failed on buckets > 1 000 objects

## v0.8.0 — 2026-02-01
### Added
- Subject browser with live message count polling
- JetStream stream and consumer listing
- Dark-mode colour palette and configurable theme

### Changed
- Migrated from nats.go v1 to jetstream v2 API
- Connection timeout reduced from 30s to 5s for snappier startup

### Fixed
- Reconnect loop on transient network errors no longer spins the CPU
- Subject filter regex was incorrectly anchored — now matches substrings

## v0.7.0 — 2026-01-05
### Added
- --user / --password authentication flags
- TLS support via --tls-ca-cert flag

### Removed
- Legacy plaintext connection fallback (security hardening)

## v0.6.0 — 2025-12-10
### Added
- Initial public release with basic subject and stream views
`

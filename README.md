A production-style API Gateway / Reverse Proxy written in Go, inspired by nginx and Kong, built from first principles with custom routing, load balancing, resilience, plugin middleware, TLS termination, hot reload, admin APIs, and nginx config compatibility.

Implemented Features
Core Gateway Runtime
Custom HTTP / HTTPS server runtime
Graceful shutdown with active request draining
Zero-downtime config reload (SIGHUP)
Atomic route table swapping
YAML + nginx config auto-detection
HTTP → HTTPS redirection
TLS termination
HTTP/2 edge support
Admin control plane
Reverse Proxy Engine
Fully custom reverse proxy implementation
Shared pooled transport layer
Request cloning and upstream rewriting
Hop-by-hop header sanitization
Forwarded header propagation
X-Forwarded-For
X-Forwarded-Proto
X-Forwarded-Host
Streaming response proxying
Trailer propagation
Retry-aware request dispatch
Load Balancing
Round Robin
Weighted Round Robin
Least Connections
Route-local upstream pools
Active connection tracking
Canary traffic splitting via weighted routing
Resilience / Fault Tolerance
Active health checks
Passive upstream failure detection
3-state circuit breaker
Closed
Open
Half-Open probing
Exponential retry backoff
Full-jitter retry strategy
Configurable retry budgets per route
Automatic upstream failover
Routing / Gateway Features
Host-based routing
Path-prefix routing
Longest-prefix route matching
Deterministic route precedence
Route-isolated upstream clusters
Per-route execution pipelines
Per-route timeout policies
Per-route retry policies
Per-route body size limits
Security & Traffic Control
Per-client IP rate limiting
Automatic stale limiter eviction
Request body enforcement (413 Payload Too Large)
TLS-secured upstream dialing
WSS upstream support

Authentication plugins:

API Key Authentication
JWT Authentication (HS256 / HS384 / HS512)
IP Restriction / CIDR filtering
Plugin System (Kong-style)

Pluggable per-route middleware architecture.

Built-in plugins:

API Key Auth
JWT Auth
IP Restriction
Request Header Transformer
Response Header Transformer
Route-local Rate Limiting

Supports custom plugin registration.

WebSocket Proxying
WebSocket upgrade detection
Upgrade handshake forwarding
101 protocol validation
Bidirectional TCP stream tunneling
TLS websocket upstream support
Context-aware tunnel cleanup
Observability
Structured request logging (zap)
Request correlation IDs
Prometheus metrics endpoint
Route-aware telemetry

Metrics include:

total requests
request latency
upstream failures
retries
circuit breaker activations
active upstream connections
Compression
Gzip response compression
Content-type aware compression
sync.Pool writer reuse
Transparent client negotiation
Admin API

Runtime gateway management endpoints:

GET  /admin/routes
GET  /admin/routes/{name}
POST /admin/reload
GET  /admin/health

Capabilities:

inspect active routes
inspect route config
trigger hot reload
operational health checks
Nginx Compatibility Layer

Native nginx config ingestion.

Supported directives:

upstream
server
location
proxy_pass
proxy_read_timeout
client_max_body_size
gzip
limit_req_zone
limit_req
proxy_set_header
ssl_certificate
listen

Includes:

hand-written lexer
recursive descent parser
internal config translator

No external parser dependencies.

Request Flow
Standard HTTP Request
Client Request
   ↓
HTTP / HTTPS Listener
   ↓
Middleware Chain
   ↓
Gateway Route Matcher
   ↓
Per-Route Plugin Chain
   ↓
Policy Enforcement
   ↓
Circuit Breaker Check
   ↓
Load Balancer
   ↓
Request Clone + Rewrite
   ↓
Shared Transport
   ↓
Upstream Response
   ↓
Retry / Backoff Decision
   ↓
Response Streaming / Compression
WebSocket Request
Client Upgrade Request
   ↓
Route Match
   ↓
Plugin / Policy Checks
   ↓
Upgrade Detection
   ↓
Client Connection Hijack
   ↓
TLS / TCP Upstream Dial
   ↓
Handshake Forwarding
   ↓
101 Validation
   ↓
Bidirectional Tunnel Relay
Testing

Comprehensive automated validation across unit, integration, performance, and SLA suites.

Coverage
Circuit breaker state transitions
Load balancing correctness
Retry logic + jitter validation
Per-IP rate limiting
Gzip behavior
JWT / API key auth
Header transformation plugins
Route matching + hot reload
End-to-end proxy correctness
Admin API behavior
Web middleware correctness
Hop-by-hop header stripping
Performance scalability
SLA compliance
Test commands
go test ./internal/... -race -timeout 60s
go test ./tests/integration/... -race -timeout 60s -v
go test ./tests/performance/... -v -timeout 120s -run TestLoad
go test ./tests/performance/... -bench=. -benchtime=5s
go test ./tests/sla/... -v -timeout 120s
go test ./... -race -timeout 120s -count=1
Performance Snapshot

Measured locally.

Concurrency	Throughput	p99 Latency
1	4,269 req/s	0.4 ms
10	14,728 req/s	1.5 ms
50	18,352 req/s	5.4 ms
100	5,469 req/s	175 ms

SLA validation:

p95 latency: 1.9 ms (target ≤ 80 ms)
p99 latency: 2.4 ms (target ≤ 150 ms)
availability healthy: ≥ 99.9%
degraded availability: ≥ 95%
hot reload error rate: 0.000%
Architecture
cmd/server
   ↓
server runtime
   ↓
router
   ↓
middleware
   ↓
gateway dispatcher
   ↓
route plugin chain
   ↓
proxy engine
   ↓
circuit breaker
   ↓
load balancer
   ↓
transport layer
   ↓
upstream services
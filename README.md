A production-style custom API Gateway / Reverse Proxy written in Go, built from first principles to explore the internals of modern Layer 7 traffic infrastructure.

## Implemented Features

### Core Server Runtime

* Config-driven HTTP/HTTPS server runtime
* Graceful shutdown (`SIGINT`, `SIGTERM`)
* Read / write / idle timeout controls
* HTTP → HTTPS redirection
* TLS termination
* HTTP/2 edge support
* Self-contained server lifecycle management

---

### Reverse Proxy Engine

* Fully custom reverse proxy implementation
* Shared connection transport with pooling
* Request cloning and upstream request rewriting
* RFC-compliant hop-by-hop header stripping
* Forwarded header propagation:

  * `X-Forwarded-For`
  * `X-Forwarded-Proto`
  * `X-Forwarded-Host`
* Streaming response proxying
* Trailer propagation support
* Retry-aware request dispatch

---

### Load Balancing

* Round Robin
* Weighted Round Robin
* Least Connections
* Route-local upstream pools
* Active connection tracking

---

### Resilience / Fault Tolerance

* Active health checks
* Passive upstream failure detection
* Basic circuit breaker protection
* Automatic failover to healthy upstreams
* Safe retry support for replay-safe HTTP methods
* Upstream isolation via per-route balancing

---

### Routing / Gateway Features

* Host-based routing
* Path-prefix routing
* Longest-prefix route matching
* Deterministic route precedence
* Per-route upstream clusters
* Independent route execution pipelines

---

### WebSocket Support

* WebSocket upgrade proxying
* Bidirectional TCP stream tunneling
* WSS upstream support
* TLS-aware websocket upstream dialing
* Upgrade handshake validation

---

### Observability

* Structured request logging (`zap`)
* Request correlation IDs
* Prometheus metrics endpoint
* Custom gateway metrics:

  * total requests
  * request latency
  * upstream failures
  * retries
  * circuit breaker activations
  * active upstream connections
* Route-aware telemetry labels

---

### Middleware

* Panic recovery
* Rate limiting
* Timeout protection
* CORS support
* Request tracing middleware

---

### Security / Edge Features

* TLS certificate loading
* HTTPS edge termination
* HTTP/2 ALPN negotiation
* Secure upstream TLS dialing
* WebSocket secure tunneling support

---

## Request Execution Flow

### Standard HTTP Request

```text
Client Request
   ↓
HTTP/HTTPS Listener
   ↓
Middleware Chain
   ↓
Gateway Route Matcher
   ↓
Route-specific Proxy Engine
   ↓
Load Balancer Selection
   ↓
Health / Circuit Validation
   ↓
Request Clone + Header Rewrite
   ↓
Shared Transport Dispatch
   ↓
Upstream Response
   ↓
Retry Decision (if eligible)
   ↓
Response Streaming to Client
```

---

### WebSocket Request

```text
Client Upgrade Request
   ↓
Gateway Route Match
   ↓
Upgrade Detection
   ↓
Client Connection Hijack
   ↓
Upstream TCP/TLS Dial
   ↓
Handshake Forwarding
   ↓
101 Upgrade Validation
   ↓
Bidirectional Stream Relay
```

---

## Current Architecture

```text
cmd/server
   ↓
server runtime
   ↓
router
   ↓
middleware pipeline
   ↓
gateway dispatcher
   ↓
route engine
   ↓
load balancer
   ↓
transport layer
   ↓
upstream services
```

---

## Current Scope

Implemented as a **production-style gateway core**, with foundational support for:

* reverse proxying
* API gateway routing
* load balancing
* resilience primitives
* websocket proxying
* HTTPS edge serving
* observability

---

## Planned Next Steps

* JWT / API key authentication
* Config hot reload
* Request / response transformation
* Plugin architecture
* Caching layer
* Distributed rate limiting
* Service discovery
* OpenTelemetry tracing
* gRPC proxying
* HTTP/3 / QUIC
* Advanced circuit breaker states
* Smooth weighted load balancing
* Admin control plane


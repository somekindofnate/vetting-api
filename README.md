# Vetting API 🛡️

**A high-availability, out-of-band SaaS vetting service.**

The Vetting API allows platforms to pass customer metadata via server-to-server webhook or direct API request and receive a calculated "Risk Score." Unlike client-side fraud tools that rely on JavaScript device fingerprinting, this engine operates as a server-side intelligence aggregator, utilizing metadata (IP, email, phone) to identify synthetic identities, automated bot traffic, and list-bombing patterns.

By consolidating primitive API calls (WHOIS, IP proxies, Telecom carrier lookup, and Deliverability checks) into a single endpoint, the service abstracts infrastructure complexity, asynchronous concurrency management, and scoring heuristics.

---

## 🏗️ System Architecture & Tech Stack

Designed for high concurrency, the system decouples the ingestion of webhook firehoses from the I/O latency of external API checks.

* **API Gateway:** Written in **Go (Golang)** using the `labstack/echo` framework.
* **OpenAPI Validation:** Request/Response models are enforced at the middleware layer using `oapi-codegen`. *(Note: Strict regex format validation for strings like emails is currently disabled for broader ingestion).*
* **Async Queues:** Ingestion tasks are handed off to **Redis** to immediately free up the HTTP connection.
* **Worker Engine:** Background workers that process the Redis queue and fan-out HTTP requests to OSINT and primitive APIs.

### The Request Lifecycle
1. **Ingestion:** The client sends a POST request with customer metadata (accepts single objects or batch arrays of up to 500 items).
2. **202 Accepted:** To prevent client-side timeout drops, the API immediately returns an HTTP 202 Accepted with a unique `job_id`. The task is queued in Redis for asynchronous processing.
3. **Waterfall Logic:** To optimize operational costs, checks execute in a waterfall sequence:
   * *Tier 1:* Zero-cost internal checks (Regex, Native DNS/MX check, Cross-tenant velocity cache).
   * *Tier 2:* Paid deliverability & IP intelligence checks.
   * *Tier 3:* Higher-latency Telecom/OSINT checks (executed conditionally).
4. **Enrichment Delivery:** Once the composite Risk Score is calculated, the system POSTs the result back to the client's provided `webhook_url`. A fallback `GET` endpoint also allows firewalled clients to poll for the completed status.

---

## 📁 Repository Structure

```text
github.com/somekindofnate/vetting-api/
├── api/             # openapi.yaml spec and auto-generated Go types/handlers
├── cmd/             # Application entry points
│   └── server/      # main.go (Echo server initialization)
├── internal/        # Private application and domain logic
│   └── vetting/     # Async workers, scoring algorithms, and Redis interfaces
├── go.mod           # Go module definitions
└── go.sum           # Dependency checksums
```

---

## 🚀 Getting Started

### Prerequisites
* [Go 1.21+](https://go.dev/doc/install)
* `oapi-codegen` CLI installed globally
* **Redis** (Required for async task queuing)

### 1. Install Redis
The API relies on a local Redis server to queue incoming validation jobs. You can find the full setup instructions in the [Official Redis Installation Guide](https://redis.io/docs/latest/operate/oss_and_stack/install/install-stack/).

**Quick Start with Docker (Recommended):**
```bash
docker run -d --name redis -p 6379:6379 redis
```

**MacOS (via Homebrew):**
```bash
brew install redis
brew services start redis
```

### 2. Install Project Dependencies
```bash
# Clone the repository
git clone https://github.com/somekindofnate/vetting-api.git
cd vetting-api

# Download Go module dependencies
go mod tidy
```

### 3. Generate OpenAPI Code (If YAML is updated)
If you make changes to `api/openapi.yaml`, regenerate the Go boilerplate:
```bash
oapi-codegen -generate types,server,spec -package api api/openapi.yaml > api/api.gen.go
```

### 4. Run the Server
Ensure your Redis instance is running, then start the Go API:
```bash
go run cmd/server/main.go
```
The server will start on `http://localhost:8080`.

---

## 📖 Interactive Documentation

The API embeds a self-hosted Redoc UI directly from the OpenAPI specification. 
With the server running, navigate your browser to the root path:

**👉 `http://localhost:8080/`**

To view the raw JSON specification:
**👉 `http://localhost:8080/openapi.json`**

---

## 🧪 Testing the API

### 1. Submit a Single Vetting Job
To simulate a platform sending a user for vetting, send a POST request to the `/v1/vetting` endpoint.

**Request:**
```bash
curl -X POST http://localhost:8080/v1/vetting \
  -H "Content-Type: application/json" \
  -d '{
    "email": "johndoe@gmail.com",
    "ip_address": "172.56.21.89",
    "phone": "+14045550199",
    "webhook_url": "https://api.clientdomain.com/v1/vetting-callback"
  }'
```

**Response:**
```json
{
  "job_id": "vett_8f72a9b3c4e5",
  "status": "queued",
  "created_at": "2026-05-20T21:41:02Z",
  "check_status_url": "https://api.yourdomain.com/v1/vetting/vett_8f72a9b3c4e5"
}
```

### 2. Submit a Batch of Jobs
You can submit up to 500 users at once by passing an array of objects to the batch endpoint.

**Request:**
```bash
curl -X POST http://localhost:8080/v1/vetting/batch \
  -H "Content-Type: application/json" \
  -d '[
    {
      "email": "alice@gmail.com",
      "ip_address": "192.168.1.1"
    },
    {
      "email": "bob@yahoo.com",
      "ip_address": "10.0.0.5"
    }
  ]'
```

### 3. Retrieve a Completed Job (Polling)
If you aren't using webhooks, you can manually check the status of a job. 

*Note: The current boilerplate is hardcoded to return a mock response. It looks up the provided ID in Redis and generates a static enrichment payload if found.*

**Request:**
```bash
# Replace 'vett_abc123' with the actual ID returned from your POST request
curl -X GET http://localhost:8080/v1/vetting/vett_abc123
```

**Response:**
```json
{
  "enrichment": {
    "identity": {
      "email_submitted": "johndoe@gmail.com",
      "possible_name": "John Doe"
    },
    "network": {
      "asn_owner": "Comcast Cable",
      "ip_vpn": false
    }
  },
  "job_id": "vett_abc123",
  "recommendation": "ALLOW",
  "risk_score": 12,
  "status": "completed"
}
```

---
*Developed by the Core Engineering Team.*

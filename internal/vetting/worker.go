package vetting

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/somekindofnate/vetting-api/api"
)

// WorkerEngine holds our shared dependencies (Redis, external API clients, DBs)
type WorkerEngine struct {
	rdb *redis.Client
	// Future: ipqsClient *ipqs.Client
	// Future: twilioClient *twilio.Client
}

// NewWorkerEngine acts as the constructor for our worker
func NewWorkerEngine(rdb *redis.Client) *WorkerEngine {
	return &WorkerEngine{
		rdb: rdb,
	}
}

// ProcessJob is the main entry point for a single vetting task.
// It executes the waterfall logic and calculates the final risk score.
func (w *WorkerEngine) ProcessJob(ctx context.Context, jobID string, payloadBytes []byte) error {
	// 1. Unmarshal the raw JSON payload from Redis
	var req api.VettingRequest
	if err := json.Unmarshal(payloadBytes, &req); err != nil {
		return fmt.Errorf("failed to decode job %s: %w", jobID, err)
	}

	log.Printf("[Worker] Starting Job: %s | Target: %s\n", jobID, req.Email)

	// 2. Initialize tracking variables
	riskScore := 0
	enrichment := make(map[string]interface{})

	// ---------------------------------------------------------
	// TIER 1: ZERO-COST INTERNAL CHECKS
	// ---------------------------------------------------------
	err := w.runTier1Checks(ctx, &req, &riskScore, enrichment)
	if err != nil {
		// If Tier 1 fails critically (e.g., badly formatted email), we can abort early to save money
		log.Printf("[Worker] %s failed Tier 1: %v\n", jobID, err)
		return w.finalizeJob(ctx, jobID, 100, "DENY", enrichment)
	}

	// ---------------------------------------------------------
	// TIER 2: STANDARD ENRICHMENT (Paid API Checks)
	// ---------------------------------------------------------
	// Only spend money if Tier 1 didn't already flag them as a definite bot
	if riskScore < 80 {
		w.runTier2Checks(ctx, &req, &riskScore, enrichment)
	}

	// ---------------------------------------------------------
	// TIER 3: DEEP OSINT / TELECOM (Expensive API Checks)
	// ---------------------------------------------------------
	// Only spend Twilio money if we are on the fence (e.g., suspicious IP but good email)
	// or if the client is paying for the Enterprise tier.
	if riskScore > 30 && riskScore < 80 && req.Phone != nil {
		w.runTier3Checks(ctx, &req, &riskScore, enrichment)
	}

	// ---------------------------------------------------------
	// FINALIZE & DELIVER
	// ---------------------------------------------------------
	recommendation := "ALLOW"
	if riskScore >= 80 {
		recommendation = "DENY"
	} else if riskScore >= 50 {
		recommendation = "MANUAL_REVIEW"
	}

	return w.finalizeJob(ctx, jobID, riskScore, recommendation, enrichment)
}

// runTier1Checks handles local Regex, Native MX lookups, and Redis velocity caching.
func (w *WorkerEngine) runTier1Checks(ctx context.Context, req *api.VettingRequest, score *int, data map[string]interface{}) error {
	log.Println(" -> Executing Tier 1 (Internal Rules)...")

	// Stub: Check if email domain has valid MX records
	// Stub: Check Redis to see if this IP submitted 500 requests in the last minute

	data["velocity"] = map[string]string{"status": "normal"}
	return nil
}

// runTier2Checks calls external providers like IPQualityScore and Kickbox.
func (w *WorkerEngine) runTier2Checks(ctx context.Context, req *api.VettingRequest, score *int, data map[string]interface{}) {
	log.Println(" -> Executing Tier 2 (IP & Email Intel)...")

	// Stub: Make HTTP GET to IPQualityScore
	if req.IpAddress != nil {
		data["network"] = map[string]interface{}{
			"is_vpn": false,
			"proxy":  false,
			"asn":    "Comcast",
		}
	}
}

// runTier3Checks calls expensive OSINT APIs like Twilio Lookup.
func (w *WorkerEngine) runTier3Checks(ctx context.Context, req *api.VettingRequest, score *int, data map[string]interface{}) {
	log.Println(" -> Executing Tier 3 (Telecom OSINT)...")

	// Stub: Make HTTP GET to Twilio
	data["telecom"] = map[string]interface{}{
		"line_type": "mobile", // Good! If it was "voip", we'd increase the risk score
		"carrier":   "Verizon",
	}
}

// finalizeJob compiles the results, saves them back to Redis, and triggers the Webhook.
func (w *WorkerEngine) finalizeJob(ctx context.Context, jobID string, score int, rec string, enrichment map[string]interface{}) error {
	result := api.VettingResult{
		JobId:          &jobID,
		Status:         func(s string) *string { return &s }("completed"),
		RiskScore:      &score,
		Recommendation: &rec,
		Enrichment:     &enrichment,
	}

	// 1. Serialize the final result
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}

	// 2. Overwrite the original pending request in Redis with the completed result
	// The GET polling endpoint in app.go will now pull THIS data instead!
	err = w.rdb.Set(ctx, jobID, resultBytes, 0).Err()
	if err != nil {
		log.Printf("Failed to save completed job %s to Redis: %v\n", jobID, err)
		return err
	}

	// 3. TODO: Fire the HTTP POST to the client's webhook_url here

	log.Printf("[Worker] Job %s Complete! Score: %d (%s)\n", jobID, score, rec)
	return nil
}

package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"

	middleware "github.com/oapi-codegen/echo-middleware"
	"github.com/somekindofnate/vetting-api/api"
)

type Server struct{}

func (s *Server) SubmitVettingJob(ctx echo.Context) error {
	jobID := fmt.Sprintf("vett_%s", uuid.New().String()[:12])
	resp := api.VettingResponse{
		JobId:          &jobID,
		Status:         func(s string) *string { return &s }("queued"),
		CreatedAt:      func(t time.Time) *time.Time { return &t }(time.Now().UTC()),
		CheckStatusUrl: func(s string) *string { return &s }(fmt.Sprintf("https://api.sentinelapi.com/v1/vetting/%s", jobID)),
	}
	return ctx.JSON(http.StatusAccepted, resp)
}

func (s *Server) GetVettingJob(ctx echo.Context, jobId string) error {
	// In production, you would query PostgreSQL or Redis here using the jobId.
	// If it didn't exist, you would return:
	// return ctx.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})

	// For our boilerplate, we will mock a "completed" response
	// containing the enriched data.
	if jobId != "abc123" {
		return ctx.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
	}
	riskScore := 12
	recommendation := "ALLOW"

	// Create a mock enrichment map
	enrichment := map[string]interface{}{
		"identity": map[string]interface{}{
			"possible_name": "John Doe",
		},
		"network": map[string]interface{}{
			"is_vpn":    false,
			"asn_owner": "Comcast Cable",
		},
	}

	resp := api.VettingResult{
		JobId:          &jobId,
		Status:         func(str string) *string { return &str }("completed"),
		RiskScore:      &riskScore,
		Recommendation: &recommendation,
		Enrichment:     &enrichment,
	}

	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) SubmitBatchVettingJobs(ctx echo.Context) error {
	// The middleware has already validated that this is an array
	// of VettingRequests and that it contains 500 or fewer items.
	var reqs []api.VettingRequest
	if err := ctx.Bind(&reqs); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid batch payload"})
	}

	// Pre-allocate the slice capacity for performance based on the batch size
	responses := make([]api.VettingResponse, 0, len(reqs))

	// Use a single timestamp for the entire batch
	now := time.Now().UTC()

	// Loop through the incoming array
	for _, _ = range reqs { // (We ignore the req data for now until Redis is hooked up)
		jobID := fmt.Sprintf("vett_%s", uuid.New().String()[:12])

		// In production, push the individual 'req' to your Redis queue here

		responses = append(responses, api.VettingResponse{
			JobId:          &jobID,
			Status:         func(s string) *string { return &s }("queued"),
			CreatedAt:      &now,
			CheckStatusUrl: func(s string) *string { return &s }(fmt.Sprintf("https://api.sentinelapi.com/v1/vetting/%s", jobID)),
		})
	}

	// Return the array of responses
	return ctx.JSON(http.StatusAccepted, responses)
}

func main() {
	e := echo.New()

	e.Use(echomiddleware.Logger())
	e.Use(echomiddleware.Recover())

	swagger, err := api.GetSwagger()
	if err != nil {
		e.Logger.Fatalf("Error loading swagger spec: %s", err)
	}
	swagger.Servers = nil

	e.GET("/openapi.json", func(c echo.Context) error {
		return c.JSON(http.StatusOK, swagger)
	})

	e.GET("/openapi.json", func(c echo.Context) error {
		// This forces kin-openapi to use its own custom JSON marshaler
		b, err := swagger.MarshalJSON()
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to marshal openapi spec")
		}
		return c.JSONBlob(http.StatusOK, b)
	})

	e.GET("/", func(c echo.Context) error {
		html := `<!DOCTYPE html>
		<html>
		  <head>
			<title>Sentinel API Documentation</title>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1">
			<link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
			<style>body { margin: 0; padding: 0; }</style>
		  </head>
		  <body>
			<redoc spec-url='/openapi.json'></redoc>
			<script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
		  </body>
		</html>`
		return c.HTML(http.StatusOK, html)
	})

	// 2. Create a specific isolated sub-group for our actual API routes
	apiGroup := e.Group("")

	// 3. Apply the OpenAPI validator middleware ONLY to this group
	apiGroup.Use(middleware.OapiRequestValidator(swagger))

	// 4. Register your generated handlers to the group instead of 'e'
	server := &Server{}
	api.RegisterHandlers(apiGroup, server)

	e.Logger.Fatal(e.Start(":8080"))
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/somekindofnate/vetting-api/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	// Alias the standard Echo middleware to 'echomiddleware'
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/getkin/kin-openapi/openapi3"

	// Alias the oapi-codegen middleware to 'middleware'
	middleware "github.com/oapi-codegen/echo-middleware"
	"github.com/somekindofnate/vetting-api/api"
	"github.com/somekindofnate/vetting-api/internal/vetting"
)

type Server struct {
	rdb *redis.Client
	db  *gorm.DB
}

func (s *Server) SubmitVettingJob(ctx echo.Context) error {
	jobID := fmt.Sprintf("vett_%s", uuid.New().String()[:12])

	pipe := s.rdb.Pipeline()

	var req api.VettingRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}

	// 1. Marshal the complex struct into a clean JSON byte array
	payloadBytes, err := json.Marshal(req)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to serialize payload"})
	}

	// 2. Use a pipeline to save the data AND notify the queue atomically
	pipe.Set(context.Background(), jobID, payloadBytes, 24*time.Hour)
	pipe.LPush(context.Background(), "vetting_queue", jobID) // Notify the worker!

	if _, err := pipe.Exec(context.Background()); err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to queue job"})
	}

	// 3. Return the 202 Accepted response
	resp := api.VettingResponse{
		JobId:          &jobID,
		Status:         func(str string) *string { return &str }("queued"),
		CreatedAt:      func(t time.Time) *time.Time { return &t }(time.Now().UTC()),
		CheckStatusUrl: func(str string) *string { return &str }(fmt.Sprintf("https://api.sentinelapi.com/v1/vetting/%s", jobID)),
	}

	return ctx.JSON(http.StatusAccepted, resp)
}

func (s *Server) GetVettingJob(ctx echo.Context, jobId string) error {
	// 1. Fetch the payload from Redis
	payloadStr, err := s.rdb.Get(context.Background(), jobId).Result()

	if err == redis.Nil {
		return ctx.JSON(http.StatusNotFound, map[string]string{"error": "Job not found"})
	} else if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Database error"})
	}

	// 2. Parse whatever is in Redis into a generic map
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &data); err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Data corruption"})
	}

	// 3. Check if the background worker has finished processing it
	if status, ok := data["status"].(string); ok && status == "completed" {
		// The worker is done! Return the real result exactly as the worker saved it.
		return ctx.JSON(http.StatusOK, data)
	}

	// 4. If it's still in the queue (or currently being processed), return a pending status
	return ctx.JSON(http.StatusOK, map[string]string{
		"job_id": jobId,
		"status": "queued",
	})
}

func (s *Server) SubmitBatchVettingJobs(ctx echo.Context) error {
	var reqs []api.VettingRequest
	if err := ctx.Bind(&reqs); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid batch payload"})
	}

	responses := make([]api.VettingResponse, 0, len(reqs))
	now := time.Now().UTC()

	// Use the globally shared Redis client via 's.rdb'
	pipe := s.rdb.Pipeline()

	for _, req := range reqs {
		jobID := fmt.Sprintf("vett_%s", uuid.New().String()[:12])

		payloadBytes, err := json.Marshal(req)
		if err != nil {
			continue
		}

		pipe.Set(context.Background(), jobID, payloadBytes, 24*time.Hour)
		pipe.LPush(context.Background(), "vetting_queue", jobID)

		responses = append(responses, api.VettingResponse{
			JobId:          &jobID,
			Status:         func(str string) *string { return &str }("queued"),
			CreatedAt:      &now,
			CheckStatusUrl: func(str string) *string { return &str }(fmt.Sprintf("https://api.yourdomain.com/v1/vetting/%s", jobID)),
		})
	}

	_, err := pipe.Exec(context.Background())
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to write batch to Redis queue"})
	}

	return ctx.JSON(http.StatusAccepted, responses)
}

func main() {
	e := echo.New()

	e.Use(echomiddleware.Logger())
	e.Use(echomiddleware.Recover())

	// 1. Connect to MySQL
	// Update with your actual MySQL credentials
	dsn := "user:password@tcp(127.0.0.1:3306)/vetting_db?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		e.Logger.Fatalf("Failed to connect to MySQL: %v", err)
	}

	// 2. Auto-migrate the schema
	// This creates the tables if they don't exist
	if err := db.AutoMigrate(&models.Organization{}, &models.User{}, &models.APIKey{}); err != nil {
		e.Logger.Fatalf("Failed to migrate database: %v", err)
	}

	openapi3.DefineStringFormat("email", openapi3.FormatOfStringForEmail)

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

	// Create a specific isolated sub-group for our actual API routes
	apiGroup := e.Group("")

	// Define the Validator Options with a custom ErrorHandler
	validatorOpts := &middleware.Options{
		// Intercept the validation errors here
		ErrorHandler: func(c echo.Context, err *echo.HTTPError) error {
			errMsg := err.Message.(string)

			// Specifically catch the ugly email regex error
			if strings.Contains(errMsg, "email") && strings.Contains(errMsg, "regular expression") {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Invalid email address format",
				})
			}

			// Clean up generic schema errors
			cleanMsg := strings.Replace(errMsg, "request body has an error: doesn't match schema #/components/schemas/VettingRequest: ", "", 1)

			// For batch errors, it uses a different prefix
			cleanMsg = strings.Replace(cleanMsg, "request body has an error: doesn't match schema : ", "", 1)

			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":   "Payload validation failed",
				"details": cleanMsg,
			})
		},
	}

	// Apply the OpenAPI validator middleware with the custom options
	apiGroup.Use(middleware.OapiRequestValidatorWithOptions(swagger, validatorOpts))

	// 1. Initialize the global Redis client once
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password
		DB:       0,  // use default DB
		Protocol: 2,
	})

	// Optional but recommended: Ping Redis to ensure it's actually running before starting the web server
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		e.Logger.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Defer closing it so it stays open for the life of the app
	defer rdb.Close()

	// 2. NOW WE CREATE THE SERVER!
	// Both db and rdb exist at this point.
	server := &Server{
		rdb: rdb,
		db:  db,
	}

	// 3. Register your generated handlers to the group
	api.RegisterHandlers(apiGroup, server)

	// 4. Initialize the worker engine
	workerEngine := vetting.NewWorkerEngine(rdb)

	// Run the queue listener in the background
	go workerEngine.StartQueueListener(context.Background())

	// 5. Boot the API
	e.Logger.Fatal(e.Start(":8080"))
}

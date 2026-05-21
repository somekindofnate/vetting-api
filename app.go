package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	
	"github.com/somekindofnate/vetting-api/api" 
	middleware "github.com/oapi-codegen/echo-middleware"
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

func main() {
	e := echo.New()

	e.Use(echomiddleware.Logger())
	e.Use(echomiddleware.Recover())

	swagger, err := api.GetSwagger()
	if err != nil {
		e.Logger.Fatalf("Error loading swagger spec: %s", err)
	}
	swagger.Servers = nil

	// -----------------------------------------------------------------
	// NEW: Expose the raw JSON spec for the documentation engine to read
	// -----------------------------------------------------------------
	e.GET("/openapi.json", func(c echo.Context) error {
		return c.JSON(http.StatusOK, swagger)
	})

	// -----------------------------------------------------------------
	// NEW: Serve Redoc UI at the root path
	// -----------------------------------------------------------------
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
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"flue-frontend/pkg/render"

	"github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	Echo    *echo.Echo
	Host    string
	Port    int
	Backend string
}

func New(host string, port int, backend string) *Server {
	return &Server{
		Echo:    echo.New(),
		Host:    host,
		Port:    port,
		Backend: backend,
	}
}

func (s *Server) Run(ctx context.Context, stop context.CancelFunc) error {
	s.setupMiddleware()
	s.Echo.HideBanner = true

	// Set the template renderer
	s.Echo.Renderer = &render.TemplateRenderer{
		Templates: template.Must(template.ParseGlob("templates/*.html")),
	}

	// Define routes
	s.Echo.GET("/", s.index) // Serve the index page
	s.Echo.POST("/", s.generate) // Handle form submission

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	go func() {
		if err := s.Echo.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Error("Failed to start server", "error", err)
			stop()
		}
	}()

	// Wait for the context to be cancelled
	<-ctx.Done()
	log.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Echo.Shutdown(ctx); err != nil {
		log.Error("Failed to shutdown server", "error", err)
		return err
	}
	log.Info("Server shutdown complete")
	return nil
}

func (s *Server) setupMiddleware() {
	s.Echo.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error == nil {
				log.Info("REQUEST", "client", c.RealIP(), "uri", v.URI, "status", v.Status)
			} else {
				log.Error("REQUEST_ERROR", "client", c.RealIP(), "uri", v.URI, "status", v.Status, "err", v.Error.Error())
			}
			return nil
		},
	}))

	s.Echo.Use(middleware.Recover())
}

func (s *Server) index(c echo.Context) error {
	return c.Render(http.StatusOK, "index.html", nil)
}

func (s *Server) generate(c echo.Context) error {
	// Extract form-encoded fields.
	prompt := c.FormValue("prompt")
	widthStr := c.FormValue("width")
	heightStr := c.FormValue("height")
	numStepsStr := c.FormValue("num_steps")
	guidanceScaleStr := c.FormValue("guidance_scale")
	seedStr := c.FormValue("seed")

	// Validate required fields.
	if prompt == "" {
		return c.String(http.StatusBadRequest, "Prompt is required")
	}
	width, err := parseFormInt(widthStr, 64, 1024)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Width is invalid: %v", err))
	}
	height, err := parseFormInt(heightStr, 64, 1024)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Height is invalid: %v", err))
	}
	numSteps, err := parseFormInt(numStepsStr, 1, 10)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Number of steps is invalid: %v", err))
	}
	guidanceScale, err := parseFormFloat(guidanceScaleStr, 0.0, 10.0)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Guidance scale is invalid: %v", err))
	}

	// Prepare the JSON payload.
	payload := map[string]any{
		"prompt":   prompt,
		"width":    width,
		"height":   height,
		"steps":    numSteps,
		"guidance": guidanceScale,
	}

	// Handle optional seed parameter.
	if seedStr != "" {
		seed, err := parseFormInt(seedStr, math.MinInt, math.MaxInt)
		if err != nil {
			c.String(http.StatusBadRequest, fmt.Sprintf("Seed is invalid: %v", err))
		}
		payload["seed"] = seed
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to encode JSON")
	}

	// Measure the time taken for the generation call.
	start := time.Now()

	// Call the local Flue server.
	resp, err := http.Post("http://localhost:8000/v1/images/generations", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to call Flue server")
	}
	defer resp.Body.Close()

	// Read the response body from the Flue server.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to read response from Flue server")
	}

	// Decode the JSON response.
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to parse JSON response")
	}

	// Compute generation time.
	genTime := time.Since(start).Seconds()

	// Prepare data for rendering the result template.
	data := map[string]any{
		"image":    result["image"],
		"gen_time": roundFloat(genTime, 2),
	}

	// Render the fragment template.
	return c.Render(http.StatusOK, "result.html", data)
}

// roundFloat rounds a float64 to a specified number of decimal places.
func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

func parseFormInt(field string, min, max int) (int, error) {
	// Helper function to parse form values as integers with min/max constraints
	valStr := field
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, fmt.Errorf("invalid integer: %s", valStr)
	}
	if val < min || val > max {
		return 0, fmt.Errorf("value out of range: %d (expected between %d and %d)", val, min, max)
	}
	return val, nil
}

func parseFormFloat(field string, min, max float64) (float64, error) {
	// Helper function to parse form values as floats with min/max constraints
	valStr := field
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float: %s", valStr)
	}
	if val < min || val > max {
		return 0, fmt.Errorf("value out of range: %f (expected between %f and %f)", val, min, max)
	}
	return val, nil
}

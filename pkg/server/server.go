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

	// Convert string fields to proper types.
	width, err := strconv.Atoi(widthStr)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid width")
	}
	height, err := strconv.Atoi(heightStr)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid height")
	}
	numSteps, err := strconv.Atoi(numStepsStr)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid number of steps")
	}
	guidanceScale, err := strconv.ParseFloat(guidanceScaleStr, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid guidance scale")
	}

	// Prepare the JSON payload.
	payload := map[string]any{
		"prompt":   prompt,
		"width":    width,
		"height":   height,
		"steps":    numSteps,
		"guidance": guidanceScale,
	}
	if seedStr != "" {
		seed, err := strconv.Atoi(seedStr)
		if err != nil {
			return c.String(http.StatusBadRequest, "Invalid seed")
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

func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

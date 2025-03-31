package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// TemplateRenderer is a custom html/template renderer for Echo.
type TemplateRenderer struct {
	templates *template.Template
}

// Render renders a template document.
func (t *TemplateRenderer) Render(w io.Writer, name string, data any, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

func main() {
	e := echo.New()

	// Set up our template renderer: it looks for templates in the "templates" directory.
	renderer := &TemplateRenderer{
		templates: template.Must(template.ParseGlob("templates/*.html")),
	}
	e.Renderer = renderer

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// GET handler: serve the index.html (the frontend form).
	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index.html", nil)
	})

	// POST handler: extract form values, call Flue, and render the result fragment.
	e.POST("/v1/images/generations", func(c echo.Context) error {
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

		body, err := ioutil.ReadAll(resp.Body)
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
	})

	// Start the server on port 8080 (or use PORT environment variable).
	port := os.Getenv("PORT")
	if port == "" {
		port = "8765"
	}
	e.Logger.Fatal(e.Start(":" + port))
}

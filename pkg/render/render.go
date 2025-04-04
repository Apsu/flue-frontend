package render

import (
	"html/template"
	"io"

	"github.com/labstack/echo/v4"
)

// TemplateRenderer is a custom html/template renderer for Echo.
type TemplateRenderer struct {
	Templates *template.Template
}

// Render renders a template document.
func (t *TemplateRenderer) Render(w io.Writer, name string, data any, c echo.Context) error {
	return t.Templates.ExecuteTemplate(w, name, data)
}

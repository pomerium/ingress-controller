package docs

import (
	"embed"
	"strings"

	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// LoadTemplates would load all templates from `./templates`
func LoadTemplates() (*template.Template, error) {
	return template.New("root").Funcs(template.FuncMap{
		"anchor": strings.ToLower,
	}).ParseFS(templateFS, "templates/*")
}

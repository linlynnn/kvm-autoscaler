package genconfig

import (
    "embed"
    "text/template"
)

//go:embed user-data.tmpl meta-data.tmpl
var tmplFS embed.FS

// LoadTemplates loads the embedded meta-data and user-data templates.
func LoadTemplates() (*template.Template, error) {
    return template.ParseFS(tmplFS, "meta-data.tmpl", "user-data.tmpl")
} 
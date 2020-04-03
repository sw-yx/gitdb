package main

import "text/template"

var packageTmpl = template.Must(template.New("package").Parse(`package main
// Code generated by gitdb embed-ui on {{.Date}}; DO NOT EDIT.
import (
	"github.com/fobilow/gitdb/v2"
)

func init() {
	//Embed Files
	{{range .Files}}
	gitdb.UI().Embed("{{.Name}}", "{{.Content}}")
	{{end}}
}
`))

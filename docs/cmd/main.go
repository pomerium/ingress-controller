// Package main is a top level command that generates CRD documentation to the stdout
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/iancoleman/strcase"

	"github.com/pomerium/ingress-controller/docs"
)

func main() {
	if err := generateMarkdown(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func generateMarkdown(w io.Writer) error {
	crd, err := docs.Load()
	if err != nil {
		return fmt.Errorf("loading CRD: %w", err)
	}

	tmpl, err := docs.LoadTemplates()
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	if err := tmpl.ExecuteTemplate(w, "header", nil); err != nil {
		return err
	}

	root := crd.Spec.Versions[0].Schema.OpenAPIV3Schema

	for _, key := range []string{"spec", "status"} {
		objects, err := docs.Flatten(key, root.Properties[key])
		if err != nil {
			return fmt.Errorf("parsing %s: %w", key, err)
		}

		fmt.Fprintf(w, "## %s\n", strcase.ToCamel(key))
		if err := tmpl.ExecuteTemplate(w, "object", objects[key]); err != nil {
			return fmt.Errorf("exec template: %w", err)
		}
		delete(objects, key)

		if err := tmpl.ExecuteTemplate(w, "objects", objects); err != nil {
			return fmt.Errorf("exec template: %w", err)
		}
	}

	return nil
}

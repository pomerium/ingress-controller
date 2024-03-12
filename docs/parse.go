package docs

import (
	"bytes"
	"fmt"
	"regexp"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/pomerium/ingress-controller/config/crd"
)

// Object is a simplified representation of JSON Schema Object
type Object struct {
	ID          string
	Description string
	Properties  map[string]*Property
}

// Property is an Object property, that may be either an atomic value, reference to an object or a map
type Property struct {
	ID          string
	Description string
	Required    bool

	ObjectOrAtomic
	Map *ObjectOrAtomic
}

// Atomic is a base type
type Atomic struct {
	Format string
	Type   string
}

// ExplainFormat returns a human readable explanation for a known format, i.e. date-time
func (a *Atomic) ExplainFormat() *string {
	if txt, ok := knownFormats[a.Format]; ok {
		return &txt
	}
	return nil
}

// ObjectOrAtomic represents either an object reference or an atomic value
type ObjectOrAtomic struct {
	// ObjectRef if set, represents a reference to an object key
	ObjectRef *string
	// Atomic if set, represents an atomic type
	Atomic *Atomic
}

// Load parses CRD document from Yaml spec
func Load() (*extv1.CustomResourceDefinition, error) {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(crd.SettingsCRD), 100)
	var spec extv1.CustomResourceDefinition
	if err := dec.Decode(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

// Flatten parses the JSON Schema and returns a flattened list of referenced objects
func Flatten(key string, src extv1.JSONSchemaProps) (map[string]*Object, error) {
	objects := make(map[string]*Object)
	if err := flatten(key, src, objects); err != nil {
		return nil, err
	}
	return objects, nil
}

var reWS = regexp.MustCompile(`(?:[\s\n\t]|\\t|\\n)+`)

func flatten(key string, src extv1.JSONSchemaProps, objects map[string]*Object) error {
	obj := &Object{
		ID:          key,
		Description: reWS.ReplaceAllString(src.Description, " "),
		Properties:  make(map[string]*Property),
	}

	atomicHandler := func(key string, prop extv1.JSONSchemaProps) (*Property, error) {
		return &Property{ObjectOrAtomic: ObjectOrAtomic{Atomic: atomic(prop)}}, nil
	}

	arrayHandler := func(key string, prop extv1.JSONSchemaProps) (*Property, error) {
		return &Property{ObjectOrAtomic: ObjectOrAtomic{Atomic: array(prop)}}, nil
	}

	typeHandler := map[string]func(key string, prop extv1.JSONSchemaProps) (*Property, error){
		"object": func(key string, prop extv1.JSONSchemaProps) (*Property, error) {
			if prop.AdditionalProperties != nil {
				// this is a map
				prop = *prop.AdditionalProperties.Schema
				if prop.Type == "object" {
					// register map value type under the name of this key
					if err := flatten(key, prop, objects); err != nil {
						return nil, err
					}
					return &Property{Map: &ObjectOrAtomic{ObjectRef: &key}}, nil
				}
				return &Property{Map: &ObjectOrAtomic{Atomic: atomic(prop)}}, nil
			}
			if err := flatten(key, prop, objects); err != nil {
				return nil, err
			}
			return &Property{ObjectOrAtomic: ObjectOrAtomic{ObjectRef: &key}}, nil
		},
		"string":  atomicHandler,
		"boolean": atomicHandler,
		"integer": atomicHandler,
		"array":   arrayHandler,
	}

	for key, prop := range src.Properties {
		fn, ok := typeHandler[prop.Type]
		if !ok {
			fmt.Printf("don't know how to handle type %s\n", prop.Type)
			continue
		}
		val, err := fn(key, prop)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		val.ID = key
		val.Description = reWS.ReplaceAllString(prop.Description, " ")
		obj.Properties[key] = val
	}

	for _, key := range src.Required {
		prop, ok := obj.Properties[key]
		if !ok {
			return fmt.Errorf("required field %s not found", key)
		}
		prop.Required = true
	}

	if _, ok := objects[key]; ok {
		return fmt.Errorf("cannot flatten: duplicate key %s", key)
	}
	objects[key] = obj

	return nil
}

func atomic(src extv1.JSONSchemaProps) *Atomic {
	return &Atomic{
		Format: src.Format,
		Type:   src.Type,
	}
}

func array(src extv1.JSONSchemaProps) *Atomic {
	return &Atomic{
		Format: src.Format,
		Type:   fmt.Sprintf("[]%s", src.Items.Schema.Type),
	}
}

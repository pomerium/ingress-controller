package v1

import (
	"fmt"
	"reflect"
	"strings"

	pom_cfg "github.com/pomerium/pomerium/config"

	"github.com/iancoleman/strcase"
)

var deprecatedFields = map[string]pom_cfg.FieldMsg{
	"idp_directory_sync": {
		DocsURL:       "https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync",
		FieldCheckMsg: pom_cfg.FieldCheckMsgRemoved,
		KeyAction:     pom_cfg.KeyActionWarn,
	},
}

// GetDeprecations returns deprecation warnings
func GetDeprecations(spec *PomeriumSpec) ([]pom_cfg.FieldMsg, error) {
	return getStructDeprecations(reflect.ValueOf(spec))
}

func getFieldDeprecations(field reflect.StructField) (*pom_cfg.FieldMsg, error) {
	reason, ok := field.Tag.Lookup("deprecated")
	if !ok {
		return nil, nil
	}
	msg, ok := deprecatedFields[reason]
	if !ok {
		return nil, fmt.Errorf("%s: not found in the lookup", reason)
	}
	jsonKey, ok := field.Tag.Lookup("json")
	if !ok {
		jsonKey = strcase.ToLowerCamel(field.Name)
	}
	jsonKey = strings.Split(jsonKey, ",")[0]
	msg.Key = jsonKey
	return &msg, nil
}

func getStructDeprecations(val reflect.Value) ([]pom_cfg.FieldMsg, error) {
	val = reflect.Indirect(val)
	if !val.IsValid() || val.IsZero() || val.Kind() != reflect.Struct {
		return nil, nil
	}

	var out []pom_cfg.FieldMsg
	for _, field := range reflect.VisibleFields(val.Type()) {
		fieldVal := reflect.Indirect(val.FieldByIndex(field.Index))
		if !fieldVal.IsValid() || fieldVal.IsZero() {
			continue
		}

		if fieldVal.Kind() == reflect.Struct {
			msgs, err := getStructDeprecations(fieldVal)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fieldVal.Type().Name(), err)
			}
			out = append(out, msgs...)
		}
		msg, err := getFieldDeprecations(field)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.Name, err)
		}
		if msg != nil {
			out = append(out, *msg)
		}
	}

	return out, nil
}

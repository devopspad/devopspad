package archive

import (
	"fmt"
	"reflect"
	"sort"
)

// SchemaKinds lists the kinds `devopspadcli schema` accepts.
func SchemaKinds() []string {
	out := []string{"archive", "note"}
	for _, k := range kinds {
		out = append(out, k.Ref)
	}
	sort.Strings(out)
	return out
}

// Schema returns a JSON Schema (draft 2020-12) generated from the Go types, so
// it always matches what the validator accepts. Cross-field and reference
// existence rules are described in x-devopspad-ref and enforced by validate.
func Schema(kind string) (map[string]any, error) {
	var v any
	switch kind {
	case "archive":
		v = Root{}
	case "note":
		v = NoteMeta{}
	default:
		k, ok := kindByRef(kind)
		if !ok {
			return nil, fmt.Errorf("unknown kind %q; kinds: %v", kind, SchemaKinds())
		}
		v = k.New()
	}
	s := schemaFor(reflect.TypeOf(v), schemaTag{})
	s["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	s["$id"] = "https://devopspad.io/schema/archive/v1alpha1/" + kind + ".json"
	if kind != "note" {
		name := "Archive"
		if k, ok := kindByRef(kind); ok {
			name = k.Kind
		}
		props := s["properties"].(map[string]any)
		props["kind"] = map[string]any{"const": name}
	}
	return s, nil
}

func schemaFor(t reflect.Type, tag schemaTag) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	s := map[string]any{}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]any{}
		var required []string
		var addFields func(reflect.Type)
		addFields = func(t reflect.Type) {
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if !f.IsExported() {
					continue
				}
				name, inline := yamlName(f)
				if inline {
					addFields(f.Type)
					continue
				}
				ft := parseTag(f.Tag.Get("schema"))
				props[name] = schemaFor(f.Type, ft)
				if ft.Required {
					required = append(required, name)
				}
			}
		}
		addFields(t)
		s["type"] = "object"
		s["properties"] = props
		s["additionalProperties"] = false
		if len(required) > 0 {
			s["required"] = required
		}
	case reflect.Slice:
		s["type"] = "array"
		item := tag
		item.Desc = ""
		s["items"] = schemaFor(t.Elem(), item)
	case reflect.Map:
		s["type"] = "object"
		s["additionalProperties"] = map[string]any{"type": "string"}
	case reflect.String:
		s["type"] = "string"
		if len(tag.Enum) > 0 {
			s["enum"] = tag.Enum
		}
		if tag.Pattern != "" {
			s["pattern"] = tag.Pattern
		}
		if tag.Const != "" {
			s["const"] = tag.Const
		}
		switch tag.Format {
		case "date":
			s["format"] = "date"
		case "url":
			s["format"] = "uri"
		}
		if len(tag.Refs) > 0 {
			s["x-devopspad-ref"] = tag.Refs
			s["pattern"] = `^[a-z]+:[^\s#@]+(#[^\s#@]+)?$`
		}
		if tag.SecretRef {
			s["x-devopspad-secretref"] = true
			s["pattern"] = `^(keychain:\S+|env:[A-Za-z_][A-Za-z0-9_]*|file:(/|~/)\S*)$`
		}
	case reflect.Bool:
		s["type"] = "boolean"
	case reflect.Int:
		s["type"] = "integer"
	case reflect.Float64:
		s["type"] = "number"
	}
	if tag.Min != nil {
		s["minimum"] = *tag.Min
	}
	if tag.Max != nil {
		s["maximum"] = *tag.Max
	}
	if tag.Desc != "" {
		s["description"] = tag.Desc
	}
	return s
}

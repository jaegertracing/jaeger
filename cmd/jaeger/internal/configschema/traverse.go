// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	durationTypeName = "duration"
	optionalPkgPath  = "go.opentelemetry.io/collector/config/configoptional"
)

// generator holds the shared state while converting Go types to JSON Schema.
type generator struct {
	docs      *docMap
	defs      map[string]*Schema
	defNameTo map[string]string // name -> typeKey, used to detect collisions.
	seen      map[string]bool   // typeKey -> already created definition.
}

func newGenerator() *generator {
	return &generator{
		docs:      newDocMap(),
		defs:      make(map[string]*Schema),
		defNameTo: make(map[string]string),
		seen:      make(map[string]bool),
	}
}

func (g *generator) typeKey(t reflect.Type) string {
	if t.PkgPath() != "" && t.Name() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.String()
}

func (g *generator) defName(t reflect.Type) string {
	key := g.typeKey(t)
	if t.PkgPath() == "" || t.Name() == "" {
		return sanitize(t.String())
	}

	// Prefer a short alias-like name, and fall back to the full package path on collisions.
	short := sanitize(lastSegment(t.PkgPath())) + "_" + t.Name()
	if existing, ok := g.defNameTo[short]; ok && existing != key {
		short = sanitize(strings.ReplaceAll(t.PkgPath(), "/", "_")) + "_" + t.Name()
	}
	g.defNameTo[short] = key
	return short
}

// ensureDef creates a definition in $defs for the provided named struct type.
// The optional default value is used to populate field defaults in the definition.
func (g *generator) ensureDef(t reflect.Type, v reflect.Value) *Schema {
	key := g.typeKey(t)
	name := g.defName(t)
	if _, ok := g.seen[key]; ok {
		return g.defs[name]
	}
	g.seen[key] = true

	s := newObjectSchema()
	s.Description = g.docs.typeDoc(key)
	if isDeprecatedDoc(s.Description) {
		s.Deprecated = true
	}
	g.defs[name] = s

	if t.Kind() == reflect.Struct {
		g.structSchema(t, v, s)
	}
	return s
}

// structSchema populates schema with the fields of the given struct type.
func (g *generator) structSchema(t reflect.Type, v reflect.Value, s *Schema) {
	if s.Description == "" {
		s.Description = g.docs.typeDoc(g.typeKey(t))
		if isDeprecatedDoc(s.Description) {
			s.Deprecated = true
		}
	}

	required := map[string]struct{}{}
	for _, existing := range s.Required {
		required[existing] = struct{}{}
	}

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		var fv reflect.Value
		if v.IsValid() {
			fv = v.Field(i)
		}

		tag := field.Tag.Get("mapstructure")
		if tag == "-" {
			continue
		}
		name, opts := parseTag(tag)

		// Embedded structs with mapstructure:",squash" are flattened into the parent.
		if field.Anonymous {
			embeddedType := field.Type
			embeddedValue := fv
			if embeddedType.Kind() == reflect.Pointer {
				if embeddedValue.IsValid() && !embeddedValue.IsNil() {
					embeddedValue = embeddedValue.Elem()
				} else {
					embeddedValue = reflect.Value{}
				}
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct {
				if opts.Contains("squash") {
					g.structSchema(embeddedType, embeddedValue, s)
					continue
				}
				if name == "" {
					name = lowerFirst(embeddedType.Name())
				}
			}
		}
		if name == "" {
			name = lowerFirst(field.Name)
		}
		if name == "" {
			continue
		}

		prop := g.typeToSchema(field.Type, fv)
		if prop == nil {
			prop = newObjectSchema()
		}

		fieldDoc := g.docs.fieldDoc(g.typeKey(t), field.Name)
		if fieldDoc != "" {
			prop.Description = fieldDoc
		}
		if isDeprecatedDoc(fieldDoc) {
			prop.Deprecated = true
		}

		if def := defaultValue(fv); def != nil {
			prop.Default = def
		}

		s.Properties[name] = prop

		if !opts.Contains("omitempty") && !opts.Contains("squash") && !prop.Deprecated && !isOptionalType(field.Type) {
			required[name] = struct{}{}
		}
	}

	req := make([]string, 0, len(required))
	for name := range required {
		req = append(req, name)
	}
	sort.Strings(req)
	s.Required = req
}

// typeToSchema converts a Go type (and optional default value) into a Schema.
func (g *generator) typeToSchema(t reflect.Type, v reflect.Value) *Schema {
	// Dereference pointers first so that nil pointers do not pollute the schema.
	if t.Kind() == reflect.Pointer {
		if v.IsValid() && !v.IsNil() {
			return g.typeToSchema(t.Elem(), v.Elem())
		}
		return g.typeToSchema(t.Elem(), reflect.Value{})
	}

	if s, ok := g.optionalSchema(t, v); ok {
		return s
	}

	if t == reflect.TypeOf(time.Duration(0)) {
		s := &Schema{Type: "string", Format: durationTypeName}
		if v.IsValid() && !v.IsZero() {
			s.Default = v.Interface().(time.Duration).String()
		}
		return s
	}

	switch t.Kind() {
	case reflect.String:
		return primitiveSchema(t, v, "string")
	case reflect.Bool:
		return primitiveSchema(t, v, "boolean")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return primitiveSchema(t, v, "integer")
	case reflect.Float32, reflect.Float64:
		return primitiveSchema(t, v, "number")
	case reflect.Slice, reflect.Array:
		itemType := t.Elem()
		var itemValue reflect.Value
		if v.IsValid() && v.Len() > 0 {
			itemValue = v.Index(0)
		}
		items := g.typeToSchema(itemType, itemValue)
		if items == nil {
			items = newObjectSchema()
		}
		return newArraySchema(items)
	case reflect.Map:
		elemType := t.Elem()
		var elemValue reflect.Value
		if v.IsValid() && v.Len() > 0 {
			// Use the first map value as a representative sample for default extraction.
			for _, val := range v.MapKeys() {
				elemValue = v.MapIndex(val)
				break
			}
		}
		additional := g.typeToSchema(elemType, elemValue)
		if additional == nil {
			additional = newObjectSchema()
		}
		return &Schema{Type: "object", AdditionalProperties: additional}
	case reflect.Struct:
		if t.PkgPath() != "" && t.Name() != "" {
			key := g.typeKey(t)
			if _, ok := g.seen[key]; !ok {
				g.ensureDef(t, v)
			}
			return &Schema{
				Ref:         "#/$defs/" + g.defName(t),
				Description: g.docs.typeDoc(key),
			}
		}
		inline := newObjectSchema()
		g.structSchema(t, v, inline)
		return inline
	case reflect.Interface:
		return newObjectSchema()
	default:
		return &Schema{Type: "string"}
	}
}

// optionalSchema handles configoptional.Optional[T] types.
func (g *generator) optionalSchema(t reflect.Type, v reflect.Value) (*Schema, bool) {
	if t.PkgPath() != optionalPkgPath || t.Name() != "Optional" {
		return nil, false
	}

	pt := reflect.PointerTo(t)
	m, ok := pt.MethodByName("Get")
	if !ok {
		return nil, false
	}
	inner := m.Type.Out(0).Elem()

	s := g.typeToSchema(inner, reflect.Zero(inner))
	if s == nil {
		s = newObjectSchema()
	}

	if v.IsValid() && v.CanAddr() {
		res := v.Addr().MethodByName("Get").Call(nil)[0]
		if !res.IsNil() {
			if def := defaultValue(res.Elem()); def != nil {
				s.Default = def
			}
		}
	}
	return s, true
}

func primitiveSchema(t reflect.Type, v reflect.Value, jsonType string) *Schema {
	s := &Schema{Type: jsonType}
	if v.IsValid() && !v.IsZero() {
		s.Default = v.Interface()
	}
	return s
}

// defaultValue returns a JSON-friendly default value or nil if none should be emitted.
func defaultValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return defaultValue(v.Elem())
	}
	if v.IsZero() {
		return nil
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan, reflect.Func:
		return nil
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			return v.Interface().(time.Duration).String()
		}
		return nil
	case reflect.Interface:
		return nil
	default:
		return v.Interface()
	}
}

func isOptionalType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.PkgPath() == optionalPkgPath && t.Name() == "Optional"
}

func parseTag(tag string) (string, tagOptions) {
	if tag == "" {
		return "", nil
	}
	parts := strings.Split(tag, ",")
	return parts[0], tagOptions(parts[1:])
}

type tagOptions []string

func (o tagOptions) Contains(option string) bool {
	for _, s := range o {
		if s == option {
			return true
		}
	}
	return false
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func lastSegment(path string) string {
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		return path[idx+1:]
	}
	return path
}

func sanitize(s string) string {
	s = strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(s)
	if s == "" {
		return "_"
	}
	if unicode.IsDigit(rune(s[0])) {
		s = "pkg_" + s
	}
	return s
}

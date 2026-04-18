package typesense

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

var schemaNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

type schemaFieldSpec struct {
	Type       string
	Facet      bool
	Sort       bool
	Index      bool
	Optional   bool
	Locale     string
	RangeIndex bool
}

func buildCollectionSchema(cfg Config, def types.IndexDefinition) (*tsapi.CollectionSchema, string, error) {
	fieldTypes, err := providerHintFieldTypes(def.ProviderHints)
	if err != nil {
		return nil, "", err
	}

	fields := fixedFieldSpecs()
	applyDefinitionFieldFlags(fields, def)
	declared := declaredFields(def)
	for field := range declared {
		if _, ok := fields[field]; ok {
			continue
		}
		typeName, ok := fieldTypes[field]
		if !ok {
			return nil, "", errs.ConfigurationError("typesense field type hint is required for declared custom fields", map[string]any{
				"index": def.Name,
				"field": field,
			})
		}
		fields[field] = schemaFieldSpec{
			Type:       typeName,
			Facet:      contains(def.FacetFields, field) || contains(def.FilterableFields, field) || def.GroupByDefault == field,
			Sort:       contains(def.SortableFields, field),
			Index:      contains(def.SearchableFields, field) || contains(def.DefaultQueryFields, field) || contains(def.HighlightFields, field),
			Optional:   true,
			RangeIndex: contains(def.FilterableFields, field) && isNumericType(typeName),
		}
	}
	for _, field := range existenceTrackedFields(def) {
		fields[existsFieldName(field)] = schemaFieldSpec{
			Type:     "bool",
			Facet:    true,
			Optional: true,
		}
	}

	fieldNames := make([]string, 0, len(fields))
	for name := range fields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)

	schemaFields := make([]tsapi.Field, 0, len(fieldNames))
	for _, name := range fieldNames {
		spec := fields[name]
		field := tsapi.Field{
			Name:     name,
			Type:     spec.Type,
			Optional: new(spec.Optional),
		}
		if spec.Facet {
			field.Facet = new(true)
		}
		if spec.Sort {
			field.Sort = new(true)
		}
		if spec.Index {
			field.Index = new(true)
		}
		if spec.RangeIndex {
			field.RangeIndex = new(true)
		}
		if spec.Locale != "" {
			field.Locale = new(spec.Locale)
		}
		schemaFields = append(schemaFields, field)
	}

	schema := &tsapi.CollectionSchema{
		Name:   collectionNameFor(cfg, def.Name),
		Fields: schemaFields,
	}
	hash := collectionSchemaHash(schema)
	return schema, hash, nil
}

func fixedFieldSpecs() map[string]schemaFieldSpec {
	return map[string]schemaFieldSpec{
		"document_id":            {Type: "string", Optional: true},
		"index":                  {Type: "string", Facet: true, Optional: true},
		"registration_key":       {Type: "string", Facet: true, Optional: true},
		"type":                   {Type: "string", Facet: true, Optional: true},
		"parent_id":              {Type: "string", Facet: true, Optional: true},
		"source_type":            {Type: "string", Facet: true, Optional: true},
		"source_id":              {Type: "string", Facet: true, Optional: true},
		"title":                  {Type: "string", Index: true, Optional: true},
		"summary":                {Type: "string", Index: true, Optional: true},
		"body":                   {Type: "string", Index: true, Optional: true},
		"url":                    {Type: "string", Optional: true},
		"anchor_url":             {Type: "string", Optional: true},
		"locale":                 {Type: "string", Facet: true, Optional: true},
		"start_ms":               {Type: "int64", Sort: true, Optional: true, RangeIndex: true},
		"end_ms":                 {Type: "int64", Sort: true, Optional: true, RangeIndex: true},
		"parent_title":           {Type: "string", Index: true, Optional: true},
		"parent_summary":         {Type: "string", Index: true, Optional: true},
		"parent_url":             {Type: "string", Optional: true},
		"parent_thumbnail":       {Type: "string", Optional: true},
		"track_kind":             {Type: "string", Facet: true, Optional: true},
		"source_format":          {Type: "string", Facet: true, Optional: true},
		"topic":                  {Type: "string[]", Facet: true, Optional: true},
		"scope_tenant_id":        {Type: "string", Facet: true, Optional: true},
		"scope_org_id":           {Type: "string", Facet: true, Optional: true},
		"scope_labels":           {Type: "string[]", Facet: true, Optional: true},
		"visibility_public":      {Type: "bool", Facet: true, Optional: true},
		"visibility_roles":       {Type: "string[]", Facet: true, Optional: true},
		"visibility_permissions": {Type: "string[]", Facet: true, Optional: true},
		"visibility_status":      {Type: "string", Facet: true, Optional: true},
	}
}

func applyDefinitionFieldFlags(fields map[string]schemaFieldSpec, def types.IndexDefinition) {
	for name, spec := range fields {
		if contains(def.FacetFields, name) || contains(def.FilterableFields, name) || def.GroupByDefault == name {
			spec.Facet = true
		}
		if contains(def.SortableFields, name) {
			spec.Sort = true
		}
		if contains(def.SearchableFields, name) || contains(def.DefaultQueryFields, name) || contains(def.HighlightFields, name) {
			spec.Index = true
		}
		if contains(def.FilterableFields, name) && isNumericType(spec.Type) {
			spec.RangeIndex = true
		}
		fields[name] = spec
	}
}

func declaredFields(def types.IndexDefinition) map[string]struct{} {
	out := map[string]struct{}{}
	for _, list := range [][]string{
		def.DefaultQueryFields,
		def.SearchableFields,
		def.FacetFields,
		def.SortableFields,
		def.FilterableFields,
		def.HighlightFields,
	} {
		for _, field := range list {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			out[field] = struct{}{}
		}
	}
	if strings.TrimSpace(def.GroupByDefault) != "" {
		out[strings.TrimSpace(def.GroupByDefault)] = struct{}{}
	}
	return out
}

func providerHintFieldTypes(hints map[string]any) (map[string]string, error) {
	out := map[string]string{}
	if len(hints) == 0 {
		return out, nil
	}
	raw, ok := hints["typesense"]
	if !ok {
		return out, nil
	}
	typedHints, ok := raw.(map[string]any)
	if !ok {
		return nil, errs.ConfigurationError("typesense provider hints must be a map", nil)
	}
	rawFieldTypes, ok := typedHints["field_types"]
	if !ok {
		return out, nil
	}
	switch v := rawFieldTypes.(type) {
	case map[string]string:
		maps.Copy(out, v)
	case map[string]any:
		for key, value := range v {
			typeName, ok := value.(string)
			if !ok {
				return nil, errs.ConfigurationError("typesense field_types values must be strings", map[string]any{"field": key})
			}
			out[key] = typeName
		}
	default:
		return nil, errs.ConfigurationError("typesense provider hint field_types must be a string map", nil)
	}
	return out, nil
}

func collectionSchemaHash(schema *tsapi.CollectionSchema) string {
	normalized := normalizedCollectionSchema{
		Name: schema.Name,
	}
	for _, field := range schema.Fields {
		if field.Name == "id" {
			continue
		}
		normalized.Fields = append(normalized.Fields, normalizedField{
			Name:       field.Name,
			Type:       field.Type,
			Facet:      ptrBool(field.Facet),
			Sort:       normalizedFieldSort(field),
			Index:      ptrBoolDefault(field.Index, true),
			Optional:   ptrBool(field.Optional),
			Locale:     ptrString(field.Locale),
			RangeIndex: ptrBool(field.RangeIndex),
		})
	}
	sort.SliceStable(normalized.Fields, func(i, j int) bool {
		return normalized.Fields[i].Name < normalized.Fields[j].Name
	})
	return hashNormalizedSchema(normalized)
}

func collectionResponseHash(schema *tsapi.CollectionResponse) string {
	normalized := normalizedCollectionSchema{
		Name: schema.Name,
	}
	for _, field := range schema.Fields {
		if field.Name == "id" {
			continue
		}
		normalized.Fields = append(normalized.Fields, normalizedField{
			Name:       field.Name,
			Type:       field.Type,
			Facet:      ptrBool(field.Facet),
			Sort:       ptrBool(field.Sort),
			Index:      ptrBool(field.Index),
			Optional:   ptrBool(field.Optional),
			Locale:     ptrString(field.Locale),
			RangeIndex: ptrBool(field.RangeIndex),
		})
	}
	sort.SliceStable(normalized.Fields, func(i, j int) bool {
		return normalized.Fields[i].Name < normalized.Fields[j].Name
	})
	return hashNormalizedSchema(normalized)
}

type normalizedCollectionSchema struct {
	Name   string            `json:"name"`
	Fields []normalizedField `json:"fields"`
}

type normalizedField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Facet      bool   `json:"facet,omitempty"`
	Sort       bool   `json:"sort,omitempty"`
	Index      bool   `json:"index,omitempty"`
	Optional   bool   `json:"optional,omitempty"`
	Locale     string `json:"locale,omitempty"`
	RangeIndex bool   `json:"range_index,omitempty"`
}

func hashNormalizedSchema(normalized normalizedCollectionSchema) string {
	body, _ := json.Marshal(normalized)
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%x", sum[:])
}

func collectionNameFor(cfg Config, index string) string {
	if cfg.CollectionNamer != nil {
		return cfg.CollectionNamer(index)
	}
	name := strings.TrimSpace(cfg.CollectionPrefix) + strings.TrimSpace(index)
	name = schemaNameSanitizer.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "search"
	}
	return name
}

func existenceTrackedFields(def types.IndexDefinition) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for field := range allowedFilterFields(def) {
		field = storageFilterField(field)
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func existsFieldName(field string) string {
	return "__exists_" + schemaNameSanitizer.ReplaceAllString(strings.TrimSpace(field), "_")
}

func contains(list []string, value string) bool {
	return slices.Contains(list, value)
}

func isNumericType(typeName string) bool {
	switch typeName {
	case "int32", "int64", "float", "bool":
		return true
	default:
		return false
	}
}

func ptrBool(value *bool) bool {
	return value != nil && *value
}

func ptrBoolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizedFieldSort(field tsapi.Field) bool {
	if field.Sort == nil && isNumericType(field.Type) {
		return true
	}
	return ptrBool(field.Sort)
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

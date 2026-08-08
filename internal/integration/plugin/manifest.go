package plugin

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

var pluginNamePattern = regexp.MustCompile(`^(?:[a-z0-9]|[a-z0-9](?:[a-z0-9.-]*[a-z0-9]))$`)

var manifestFields = map[string]struct{}{
	"$schema": {}, "name": {}, "version": {}, "description": {}, "author": {},
	"homepage": {}, "repository": {}, "license": {}, "keywords": {}, "extensions": {},
}

func loadManifest(body []byte) (Manifest, []Diagnostic, error) {
	object, err := decodeJSONObject(body)
	if err != nil {
		return Manifest{}, nil, invalidManifest("plugin.json is not a valid JSON object", err)
	}
	diagnostics := make([]Diagnostic, 0)
	unknown := make([]string, 0)
	for field := range object {
		if _, known := manifestFields[field]; !known {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		diagnostics = append(diagnostics, Diagnostic{Level: DiagnosticWarning, Code: "PLUGIN_MANIFEST_UNKNOWN_FIELD", Path: "plugin.json", Message: fmt.Sprintf("unknown top-level field %q was ignored", field)})
	}
	schema, err := decodeString(object["$schema"], "$schema")
	if err != nil || schema != ManifestSchemaURL {
		return Manifest{}, diagnostics, invalidManifest("plugin.json targets an unsupported Agent Plugins schema", err)
	}
	name, err := decodeString(object["name"], "name")
	if err != nil || !validPluginName(name) {
		return Manifest{}, diagnostics, invalidManifest("plugin.json contains an invalid plugin name", err)
	}
	manifest := Manifest{Schema: schema, Name: name}
	for _, field := range []struct {
		name   string
		target *string
	}{
		{"version", &manifest.Version}, {"description", &manifest.Description},
		{"homepage", &manifest.Homepage}, {"repository", &manifest.Repository},
		{"license", &manifest.License},
	} {
		raw, exists := object[field.name]
		if !exists {
			continue
		}
		value, err := decodeString(raw, field.name)
		if err != nil {
			return Manifest{}, diagnostics, invalidManifest("plugin.json field "+field.name+" has an invalid type", err)
		}
		*field.target = value
	}
	if raw, exists := object["author"]; exists {
		author, err := loadAuthor(raw)
		if err != nil {
			return Manifest{}, diagnostics, invalidManifest("plugin.json author is invalid", err)
		}
		manifest.Author = &author
	}
	if raw, exists := object["keywords"]; exists {
		keywords, err := decodeStringSlice(raw, "keywords")
		if err != nil {
			return Manifest{}, diagnostics, invalidManifest("plugin.json keywords are invalid", err)
		}
		manifest.Keywords = keywords
	}
	if raw, exists := object["extensions"]; exists {
		extensions, err := decodeJSONObject(raw)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Level: DiagnosticWarning, Code: "PLUGIN_MANIFEST_EXTENSIONS_IGNORED", Path: "plugin.json", Message: "non-object extensions field was ignored"})
		} else {
			for namespace, value := range extensions {
				var probe map[string]json.RawMessage
				if json.Unmarshal(value, &probe) != nil || probe == nil {
					return Manifest{}, diagnostics, invalidManifest(fmt.Sprintf("extension %q must be an object", namespace), nil)
				}
			}
			manifest.Extensions = extensions
		}
	}
	return manifest, diagnostics, nil
}

func loadAuthor(raw json.RawMessage) (Author, error) {
	object, err := decodeJSONObject(raw)
	if err != nil {
		return Author{}, err
	}
	author := Author{}
	for field, value := range object {
		var target *string
		switch field {
		case "name":
			target = &author.Name
		case "email":
			target = &author.Email
		case "url":
			target = &author.URL
		default:
			return Author{}, fmt.Errorf("unknown author field %q", field)
		}
		decoded, err := decodeString(value, "author."+field)
		if err != nil {
			return Author{}, err
		}
		*target = decoded
	}
	return author, nil
}

func validPluginName(value string) bool {
	return len(value) >= 1 && len(value) <= 64 && pluginNamePattern.MatchString(value) && !strings.Contains(value, "--") && !strings.Contains(value, "..")
}

func invalidManifest(message string, cause error) error {
	if cause != nil {
		message += ": " + cause.Error()
	}
	return domain.Invalid("AGENT_PLUGIN_MANIFEST_INVALID", message)
}

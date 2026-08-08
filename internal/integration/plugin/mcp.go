package plugin

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

var mcpTopLevelFields = map[string]struct{}{"$schema": {}, "mcpServers": {}}

func discoverMCPServers(root string, limits Limits) ([]MCPServer, []Diagnostic) {
	path := filepath.Join(root, "mcp.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return []MCPServer{}, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return []MCPServer{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_MCP_LOCATION_INVALID", Path: "mcp.json", Message: "mcp.json must be a non-symlink regular file"}}
	}
	body, err := readPackageFile(root, "mcp.json", limits.MaxFileBytes)
	if err != nil {
		return []MCPServer{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_MCP_READ_FAILED", Path: "mcp.json", Message: err.Error()}}
	}
	object, err := decodeJSONObject(body)
	if err != nil {
		return []MCPServer{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_MCP_INVALID", Path: "mcp.json", Message: err.Error()}}
	}
	for field := range object {
		if _, known := mcpTopLevelFields[field]; !known {
			return []MCPServer{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_MCP_INVALID", Path: "mcp.json", Message: fmt.Sprintf("unknown top-level field %q", field)}}
		}
	}
	schema, err := decodeString(object["$schema"], "$schema")
	if err != nil || schema != MCPSchemaURL {
		return []MCPServer{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_MCP_SCHEMA_UNSUPPORTED", Path: "mcp.json", Message: "mcp.json targets an unsupported or mismatched Agent Plugins schema"}}
	}
	rawServers, exists := object["mcpServers"]
	if !exists {
		return []MCPServer{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_MCP_INVALID", Path: "mcp.json", Message: "mcpServers is required"}}
	}
	serversObject, err := decodeJSONObject(rawServers)
	if err != nil {
		return []MCPServer{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_MCP_INVALID", Path: "mcp.json", Message: "mcpServers must be an object"}}
	}
	names := make([]string, 0, len(serversObject))
	for name := range serversObject {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]MCPServer, 0, len(names))
	diagnostics := make([]Diagnostic, 0)
	for _, name := range names {
		server, diagnostic := parseMCPServer(root, name, serversObject[name])
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			continue
		}
		servers = append(servers, server)
		if !server.Supported {
			diagnostics = append(diagnostics, Diagnostic{Level: DiagnosticUnsupported, Code: "PLUGIN_MCP_TRANSPORT_UNSUPPORTED", Path: "mcp.json", Component: name, Message: fmt.Sprintf("MCP transport %q is valid but not enabled in the first release", server.Type)})
		}
	}
	return servers, diagnostics
}

func parseMCPServer(root, name string, raw json.RawMessage) (MCPServer, *Diagnostic) {
	fail := func(message string) (MCPServer, *Diagnostic) {
		return MCPServer{}, &Diagnostic{Level: DiagnosticError, Code: "PLUGIN_MCP_SERVER_INVALID", Path: "mcp.json", Component: name, Message: message}
	}
	object, err := decodeJSONObject(raw)
	if err != nil {
		return fail("server entry must be a JSON object: " + err.Error())
	}
	typeName, err := decodeString(object["type"], "type")
	if err != nil {
		return fail("server type is required")
	}
	switch typeName {
	case "stdio":
		return parseStdioServer(root, name, object, fail)
	case "streamable-http", "sse":
		return parseRemoteServer(name, typeName, object, fail)
	default:
		return fail(fmt.Sprintf("unknown MCP transport %q", typeName))
	}
}

type mcpFailure func(string) (MCPServer, *Diagnostic)

func parseStdioServer(root, name string, object map[string]json.RawMessage, fail mcpFailure) (MCPServer, *Diagnostic) {
	if field := unknownField(object, "type", "command", "args", "env", "cwd"); field != "" {
		return fail(fmt.Sprintf("unknown stdio field %q", field))
	}
	command, err := decodeString(object["command"], "command")
	if err != nil || !validCommandToken(command) {
		return fail("stdio command must be one bare executable token or a ./ plugin-relative path")
	}
	if strings.HasPrefix(command, "./") {
		if err := validatePackageCommand(root, command); err != nil {
			return fail(err.Error())
		}
	}
	server := MCPServer{Name: name, Type: "stdio", Command: command, Supported: true}
	if raw, exists := object["args"]; exists {
		server.Args, err = decodeStringSlice(raw, "args")
		if err != nil {
			return fail(err.Error())
		}
	}
	if raw, exists := object["env"]; exists {
		server.Env, err = decodeStringMap(raw, "env")
		if err != nil {
			return fail(err.Error())
		}
		for key := range server.Env {
			if strings.EqualFold(key, "PLUGIN_ROOT") || strings.EqualFold(key, "PLUGIN_DATA") {
				return fail("stdio env must not override PLUGIN_ROOT or PLUGIN_DATA")
			}
		}
	}
	if raw, exists := object["cwd"]; exists {
		server.CWD, err = decodeString(raw, "cwd")
		if err != nil {
			return fail(err.Error())
		}
		if err := validateConfiguredCWD(root, server.CWD); err != nil {
			return fail(err.Error())
		}
	}
	return server, nil
}

func parseRemoteServer(name, typeName string, object map[string]json.RawMessage, fail mcpFailure) (MCPServer, *Diagnostic) {
	if field := unknownField(object, "type", "url", "headers"); field != "" {
		return fail(fmt.Sprintf("unknown %s field %q", typeName, field))
	}
	endpoint, err := decodeString(object["url"], "url")
	if err != nil {
		return fail("remote MCP url is required")
	}
	if err := validateRemoteURL(endpoint); err != nil {
		return fail(err.Error())
	}
	server := MCPServer{Name: name, Type: typeName, URL: endpoint, Supported: false}
	if raw, exists := object["headers"]; exists {
		server.Headers, err = decodeStringMap(raw, "headers")
		if err != nil {
			return fail(err.Error())
		}
		seen := map[string]struct{}{}
		for key, value := range server.Headers {
			normalized := strings.ToLower(key)
			if _, duplicate := seen[normalized]; duplicate {
				return fail("remote MCP headers contain a case-insensitive duplicate")
			}
			seen[normalized] = struct{}{}
			if !validHTTPHeaderName(key) || strings.ContainsAny(value, "\r\n") {
				return fail("remote MCP headers contain an invalid field")
			}
		}
	}
	return server, nil
}

func unknownField(object map[string]json.RawMessage, allowed ...string) string {
	known := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		known[field] = struct{}{}
	}
	fields := make([]string, 0)
	for field := range object {
		if _, ok := known[field]; !ok {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func validCommandToken(command string) bool {
	if command == "" || command != strings.TrimSpace(command) || strings.Contains(command, "${PLUGIN_") {
		return false
	}
	if strings.IndexFunc(command, unicode.IsSpace) >= 0 {
		return false
	}
	if strings.HasPrefix(command, "./") {
		return isSafeSlashRelative(command)
	}
	return !strings.ContainsAny(command, `/\\`)
}

func validatePackageCommand(root, command string) error {
	relative := strings.TrimPrefix(command, "./")
	path := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("plugin-relative command does not resolve: %s", command)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || !pathWithin(root, resolved) {
		return fmt.Errorf("plugin-relative command must resolve to a regular file within PLUGIN_ROOT: %s", command)
	}
	return nil
}

func validateConfiguredCWD(root, cwd string) error {
	var path string
	switch {
	case strings.HasPrefix(cwd, "./"):
		if !isSafeSlashRelative(cwd) {
			return fmt.Errorf("cwd escapes PLUGIN_ROOT")
		}
		path = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cwd, "./")))
	case cwd == "${PLUGIN_ROOT}":
		path = root
	case strings.HasPrefix(cwd, "${PLUGIN_ROOT}/"):
		relative := strings.TrimPrefix(cwd, "${PLUGIN_ROOT}/")
		if !isSafeSlashSuffix(relative) {
			return fmt.Errorf("cwd escapes PLUGIN_ROOT")
		}
		path = filepath.Join(root, filepath.FromSlash(relative))
	case cwd == "${PLUGIN_DATA}":
		return nil
	case strings.HasPrefix(cwd, "${PLUGIN_DATA}/"):
		if !isSafeSlashSuffix(strings.TrimPrefix(cwd, "${PLUGIN_DATA}/")) {
			return fmt.Errorf("cwd escapes PLUGIN_DATA")
		}
		return nil
	default:
		return fmt.Errorf("cwd must be ./, PLUGIN_ROOT, or PLUGIN_DATA rooted")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("cwd does not resolve to a directory within PLUGIN_ROOT")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() || !pathWithin(root, resolved) {
		return fmt.Errorf("cwd does not resolve to a directory within PLUGIN_ROOT")
	}
	return nil
}

func isSafeSlashRelative(value string) bool {
	return strings.HasPrefix(value, "./") && isSafeSlashSuffix(strings.TrimPrefix(value, "./"))
}

func isSafeSlashSuffix(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	return clean != ".." && !strings.HasPrefix(clean, "../")
}

func validateRemoteURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("remote MCP url must be an absolute HTTP(S) URL without userinfo or fragment")
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && host != "localhost" {
		address := net.ParseIP(host)
		if address == nil || !address.IsLoopback() {
			return fmt.Errorf("non-loopback remote MCP url must use HTTPS")
		}
	}
	return nil
}

func validHTTPHeaderName(value string) bool {
	if value == "" {
		return false
	}
	const separators = `()<>@,;:\"/[]?={} ` + "\t"
	for _, character := range value {
		if character <= 31 || character >= 127 || strings.ContainsRune(separators, character) {
			return false
		}
	}
	return true
}

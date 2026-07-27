package cli

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/capabilitycatalog"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

func (r *Root) artifactCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "Register, inspect, export, download, and safely open artifacts"}
	var scriptID string
	list := &cobra.Command{Use: "list", Short: "List server-computed presentations for a ScriptVersion", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.ArtifactPresentation
		if err := client.Dispatch(cmd.Context(), "artifact.list", map[string]any{"script_id": scriptID}, &result); err != nil {
			return err
		}
		return r.writeOK("artifact.list", result)
	}}
	list.Flags().StringVar(&scriptID, "script", "", "script version ID")

	presentation := &cobra.Command{Use: "presentation <artifact-id>", Args: cobra.ExactArgs(1), Short: "Read the server-computed presentation tier and allowed actions", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ArtifactPresentation
		if err := client.Dispatch(cmd.Context(), "artifact.presentation", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("artifact.presentation", result)
	}}

	var format, exportScriptID string
	export := &cobra.Command{Use: "export <approved-snapshot-id>", Args: cobra.ExactArgs(1), Short: "Create an export from a client-approved snapshot", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Artifact
		if err := client.Dispatch(cmd.Context(), "artifact.export", map[string]any{"snapshot_id": args[0], "script_id": exportScriptID, "format": format}, &result); err != nil {
			return err
		}
		return r.writeOK("artifact.export", result)
	}}
	export.Flags().StringVar(&format, "format", "json", "markdown, xlsx, or json")
	export.Flags().StringVar(&exportScriptID, "script", "", "script ID when the snapshot contains multiple scripts")

	var deliveryScriptID string
	delivery := &cobra.Command{Use: "package <approved-snapshot-id>", Args: cobra.ExactArgs(1), Short: "Create a three-format DeliveryPackage", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.DeliveryPackage
		if err := client.Dispatch(cmd.Context(), "delivery.create", map[string]any{"snapshot_id": args[0], "script_id": deliveryScriptID}, &result); err != nil {
			return err
		}
		return r.writeOK("delivery.create", result)
	}}
	delivery.Flags().StringVar(&deliveryScriptID, "script", "", "script ID when the snapshot contains multiple scripts")
	var deliveryProjectID string
	deliveryList := &cobra.Command{Use: "packages", Short: "List immutable DeliveryPackages", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.DeliveryPackage
		if err := client.Dispatch(cmd.Context(), "delivery.list", map[string]any{"project_id": deliveryProjectID}, &result); err != nil {
			return err
		}
		return r.writeOK("delivery.list", result)
	}}
	deliveryList.Flags().StringVar(&deliveryProjectID, "project", "", "project ID")
	_ = deliveryList.MarkFlagRequired("project")
	deliveryShow := &cobra.Command{Use: "package-show <delivery-package-id>", Args: cobra.ExactArgs(1), Short: "Show a DeliveryPackage manifest", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.DeliveryPackage
		if err := client.Dispatch(cmd.Context(), "delivery.show", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("delivery.show", result)
	}}

	var out string
	download := &cobra.Command{Use: "download <artifact-id>", Args: cobra.ExactArgs(1), Short: "Download a server-hosted artifact to an explicit path", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result struct {
			Artifact      domain.Artifact `json:"artifact"`
			ContentBase64 string          `json:"content_base64"`
		}
		if err := client.Dispatch(cmd.Context(), "artifact.download", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		data, err := base64.StdEncoding.DecodeString(result.ContentBase64)
		if err != nil {
			return err
		}
		path := out
		if path == "" {
			path = result.Artifact.FileName
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		return r.writeOK("artifact.download", map[string]any{"artifact_id": result.Artifact.ID, "path": path, "byte_size": len(data), "sha256": result.Artifact.SHA256})
	}}
	download.Flags().StringVar(&out, "out", "", "output file path")

	register := r.artifactRegisterCommand()
	open := r.artifactOpenCommand()
	openStatus := &cobra.Command{Use: "open-status <open-request-id>", Args: cobra.ExactArgs(1), Short: "Read a local-open request state", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ArtifactOpenRequest
		if err := client.Dispatch(cmd.Context(), "artifact.open.status", map[string]any{"open_request_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("artifact.open.status", result)
	}}
	cmd.AddCommand(list, presentation, export, delivery, deliveryList, deliveryShow, download, register, open, openStatus)
	return cmd
}

func (r *Root) artifactRegisterCommand() *cobra.Command {
	var scriptVersionID, schemaID, mediaType, capabilityID, capabilityVersion, capabilityDigest string
	var metadataValues []string
	var dryRun bool
	command := &cobra.Command{Use: "register <file>", Args: cobra.ExactArgs(1), Short: "Register a local extension artifact without uploading its bytes", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		projectID, err := localconfig.ResolveProject(r.projectID, cfg)
		if err != nil {
			return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "请用 --project、CONTENTCLOUD_PROJECT_ID 或本机项目绑定指定项目")
		}
		path, info, sha, err := inspectLocalArtifact(args[0])
		if err != nil {
			return err
		}
		if mediaType == "" {
			mediaType = mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
			if mediaType == "" {
				mediaType = "application/octet-stream"
			}
		}
		metadata, err := parseArtifactMetadata(metadataValues)
		if err != nil {
			return err
		}
		envelope := domain.ExtensionArtifactEnvelopeV1{EnvelopeVersion: domain.ArtifactEnvelopeVersion, ProjectID: projectID, ScriptVersionID: scriptVersionID, Capability: domain.ArtifactCapabilityRef{ID: capabilityID, Version: capabilityVersion, Digest: capabilityDigest}, SchemaID: schemaID, MediaType: mediaType, SHA256: sha, Size: info.Size(), Renditions: []domain.ArtifactRenditionRef{}, Metadata: metadata}
		if err := domain.ValidateExtensionArtifactEnvelope(envelope); err != nil {
			return err
		}
		deviceToken, err := localconfig.DeviceToken(cfg.DeviceID)
		if err != nil {
			return &domain.Error{Type: "credential", Subtype: "device", Code: "DEVICE_CREDENTIAL_MISSING", Message: err.Error(), ExitCode: 3}
		}
		var result app.RegisterArtifactResult
		input := app.RegisterArtifactInput{Envelope: envelope, FileName: filepath.Base(path), Capabilities: builtinCapabilities(), DryRun: dryRun}
		if err := apiclient.New(r.resolveServer(cfg), deviceToken).Dispatch(cmd.Context(), "artifact.register", input, &result); err != nil {
			return err
		}
		if !dryRun {
			if err := localconfig.SaveLocalArtifact(localconfig.LocalArtifact{ArtifactID: result.Artifact.ID, Path: path, SHA256: sha, ByteSize: info.Size()}); err != nil {
				return err
			}
		}
		return r.writeOK("artifact.register", result)
	}}
	command.Flags().StringVar(&scriptVersionID, "script", "", "immutable ScriptVersion ID")
	command.Flags().StringVar(&schemaID, "schema", "opaque/1.0", "artifact business schema ID")
	command.Flags().StringVar(&mediaType, "media-type", "", "declared MIME type; inferred from extension when omitted")
	command.Flags().StringVar(&capabilityID, "capability-id", domain.ArtifactExportCapability, "source capability ID")
	command.Flags().StringVar(&capabilityVersion, "capability-version", "1.0.0", "source capability semver")
	artifactCapability, _ := capabilitycatalog.Exact(domain.ArtifactExportCapability, Version)
	command.Flags().StringVar(&capabilityDigest, "capability-digest", artifactCapability.Digest, "source capability digest")
	command.Flags().StringArrayVar(&metadataValues, "metadata", nil, "scalar metadata in key=value form; repeatable")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate locally and on the server without registering")
	_ = command.MarkFlagRequired("script")
	return command
}

func (r *Root) artifactOpenCommand() *cobra.Command {
	var deviceID string
	var dryRun bool
	command := &cobra.Command{Use: "open <artifact-id>", Args: cobra.ExactArgs(1), Short: "Request an online source device to open an artifact locally", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		if deviceID == "" {
			var presentation domain.ArtifactPresentation
			if err := client.Dispatch(cmd.Context(), "artifact.presentation", map[string]any{"id": args[0]}, &presentation); err != nil {
				return err
			}
			if presentation.SourceDevice == nil {
				return domain.Policy("ARTIFACT_LOCAL_OPEN_UNAVAILABLE", "Artifact 没有可用来源设备", "确认来源设备在线")
			}
			deviceID = presentation.SourceDevice.ID
		}
		var result app.ArtifactOpenResult
		if err := client.Dispatch(cmd.Context(), "artifact.open", map[string]any{"id": args[0], "device_id": deviceID, "dry_run": dryRun}, &result); err != nil {
			return err
		}
		return r.writeOK("artifact.open", result)
	}}
	command.Flags().StringVar(&deviceID, "device", "", "source device ID; defaults to the registered source device")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate without creating an open request")
	return command
}

func inspectLocalArtifact(inputPath string) (string, os.FileInfo, string, error) {
	absolute, err := filepath.Abs(inputPath)
	if err != nil {
		return "", nil, "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", nil, "", domain.Invalid("ARTIFACT_FILE_INVALID", "Artifact 必须是非空普通文件")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", nil, "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", nil, "", err
	}
	return resolved, info, hex.EncodeToString(hash.Sum(nil)), nil
}

func parseArtifactMetadata(values []string) (map[string]any, error) {
	metadata := map[string]any{}
	for _, value := range values {
		key, item, found := strings.Cut(value, "=")
		if !found || strings.TrimSpace(key) == "" {
			return nil, domain.Invalid("ARTIFACT_METADATA_INVALID", "--metadata 必须使用 key=value")
		}
		metadata[strings.TrimSpace(key)] = item
	}
	return metadata, nil
}

func handlePendingArtifactOpen(ctx context.Context, client *apiclient.Client) (bool, domain.ArtifactOpenRequest, error) {
	var lease domain.ArtifactOpenLease
	if err := client.Dispatch(ctx, "artifact.open.poll", map[string]any{"capabilities": builtinCapabilities()}, &lease); err != nil {
		var domainError *domain.Error
		if errors.As(err, &domainError) && domainError.Code == "NO_TASK" {
			return false, domain.ArtifactOpenRequest{}, nil
		}
		return false, domain.ArtifactOpenRequest{}, err
	}
	var result domain.ArtifactOpenRequest
	if err := client.Dispatch(ctx, "artifact.open.finish", map[string]any{"open_request_id": lease.OpenRequestID, "state": "accepted", "reason": ""}, &result); err != nil {
		return true, result, err
	}
	local, err := localconfig.LocalArtifactByID(lease.ArtifactID)
	if err != nil {
		finishErr := client.Dispatch(ctx, "artifact.open.finish", map[string]any{"open_request_id": lease.OpenRequestID, "state": "not_available", "reason": "local_index_missing"}, &result)
		return true, result, finishErr
	}
	path, info, hash, err := inspectLocalArtifact(local.Path)
	if err != nil || path != local.Path || info.Size() != local.ByteSize || hash != local.SHA256 {
		finishErr := client.Dispatch(ctx, "artifact.open.finish", map[string]any{"open_request_id": lease.OpenRequestID, "state": "not_available", "reason": "file_changed"}, &result)
		return true, result, finishErr
	}
	if err := launchLocalArtifact(path); err != nil {
		reason := "open_failed"
		if errors.Is(err, exec.ErrNotFound) {
			reason = "launcher_unavailable"
		}
		finishErr := client.Dispatch(ctx, "artifact.open.finish", map[string]any{"open_request_id": lease.OpenRequestID, "state": "failed", "reason": reason}, &result)
		return true, result, finishErr
	}
	if err := client.Dispatch(ctx, "artifact.open.finish", map[string]any{"open_request_id": lease.OpenRequestID, "state": "opened", "reason": ""}, &result); err != nil {
		return true, result, err
	}
	return true, result, nil
}

func launchLocalArtifact(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", path)
	case "linux":
		command = exec.Command("xdg-open", path)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return exec.ErrNotFound
	}
	return command.Start()
}

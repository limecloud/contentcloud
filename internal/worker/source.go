package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/ingest"
)

type mediaProcessor interface {
	PendingMediaGenerationJobs(context.Context, int) ([]domain.MediaGenerationJob, error)
	ProcessMediaGenerationJob(context.Context, string, string) error
}

func ProcessPendingSources(ctx context.Context, service *app.Service, limit int) (int, error) {
	pending, err := service.PendingSourceRevisions(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, candidate := range pending {
		revision, claimed, err := service.ClaimSourceRevision(ctx, candidate.TenantID, candidate.ID)
		if err != nil {
			return processed, err
		}
		if !claimed {
			continue
		}
		body, err := service.SourceRevisionBytes(ctx, revision)
		if err != nil {
			_, completeErr := service.CompleteSource(ctx, app.Actor{TenantID: revision.TenantID, Type: "worker"}, revision.ID, app.CompleteSourceInput{DetectedMIME: revision.DeclaredMIME, Status: "failed", ParserVersion: ingest.ParserVersion, ErrorCode: "OBJECT_READ_FAILED"}, "")
			if completeErr != nil {
				return processed, fmt.Errorf("读取来源 %s 失败：%w；标记完成时又发生错误：%v", revision.ID, err, completeErr)
			}
			processed++
			continue
		}
		detectedMIME := ingest.DetectMIME(body)
		if detectedMIME != revision.DeclaredMIME {
			_, completeErr := service.CompleteSource(ctx, app.Actor{TenantID: revision.TenantID, Type: "worker"}, revision.ID, app.CompleteSourceInput{DetectedMIME: detectedMIME, Status: "failed", ParserVersion: ingest.ParserVersion, ErrorCode: "MIME_MISMATCH"}, "")
			if completeErr != nil {
				return processed, completeErr
			}
			processed++
			continue
		}
		if scanCode, scanErr := scanSource(ctx, revision.FileName, body); scanErr != nil {
			_, completeErr := service.CompleteSource(ctx, app.Actor{TenantID: revision.TenantID, Type: "worker"}, revision.ID, app.CompleteSourceInput{DetectedMIME: detectedMIME, Status: "failed", ParserVersion: ingest.ParserVersion, ErrorCode: scanCode}, "")
			if completeErr != nil {
				return processed, fmt.Errorf("扫描来源 %s 失败：%w；标记完成时又发生错误：%v", revision.ID, scanErr, completeErr)
			}
			processed++
			continue
		}
		result := ingest.Parse(revision.FileName, detectedMIME, body)
		evidence := make([]app.CreateEvidenceInput, 0, len(result.Evidence))
		for _, span := range result.Evidence {
			evidence = append(evidence, app.CreateEvidenceInput{LocatorKind: span.LocatorKind, Locator: span.Locator, QuoteText: span.QuoteText, OCRConfidence: span.OCRConfidence})
		}
		_, err = service.CompleteSource(ctx, app.Actor{TenantID: revision.TenantID, Type: "worker"}, revision.ID, app.CompleteSourceInput{DetectedMIME: detectedMIME, Status: result.Status, ParserVersion: ingest.ParserVersion, ErrorCode: result.ErrorCode, Evidence: evidence}, "")
		if err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func ProcessPendingMedia(ctx context.Context, service mediaProcessor, limit int) (int, error) {
	pending, err := service.PendingMediaGenerationJobs(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, job := range pending {
		if err := service.ProcessMediaGenerationJob(ctx, job.TenantID, job.ID); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func scanSource(ctx context.Context, fileName string, data []byte) (string, error) {
	required := os.Getenv("CONTENTCLOUD_REQUIRE_MALWARE_SCAN") == "1"
	binary := ""
	for _, candidate := range []string{"clamscan", "clamdscan"} {
		if path, err := exec.LookPath(candidate); err == nil {
			binary = path
			break
		}
	}
	if binary == "" {
		if required {
			return "MALWARE_SCANNER_UNAVAILABLE", fmt.Errorf("ClamAV 扫描器不可用")
		}
		return "", nil
	}
	dir, err := os.MkdirTemp("", "contentcloud-scan-*")
	if err != nil {
		return "MALWARE_SCAN_FAILED", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, filepath.Base(fileName))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "MALWARE_SCAN_FAILED", err
	}
	scanCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{"--no-summary", path}
	if strings.HasSuffix(binary, "clamdscan") {
		args = append([]string{"--fdpass", "--no-summary"}, path)
	}
	output, err := exec.CommandContext(scanCtx, binary, args...).CombinedOutput()
	if err == nil {
		return "", nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return "MALWARE_DETECTED", fmt.Errorf("检测到恶意文件：%s", strings.TrimSpace(string(output)))
	}
	return "MALWARE_SCAN_FAILED", fmt.Errorf("ClamAV 扫描失败：%w", err)
}

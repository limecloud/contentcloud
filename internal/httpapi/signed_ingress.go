package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func verifySignedIngress(w http.ResponseWriter, r *http.Request, secret []byte, codePrefix, subject string) ([]byte, string, *domain.Error) {
	timestamp := strings.TrimSpace(r.Header.Get("X-ContentCloud-Timestamp"))
	provided := strings.TrimSpace(r.Header.Get("X-ContentCloud-Signature"))
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || timestamp == "" || provided == "" || absDuration(time.Since(time.Unix(seconds, 0))) > providerIngressClockSkew {
		return nil, "", domain.Invalid(codePrefix+"_REPLAY", subject+" ingress 时间戳无效或已超出重放保护窗口")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, providerIngressBodyLimit+1))
	if err != nil || int64(len(body)) > providerIngressBodyLimit {
		return nil, "", domain.Invalid(codePrefix+"_BODY_TOO_LARGE", subject+" ingress 请求体超过大小限制")
	}
	hash := sha256.Sum256(body)
	digestHex := hex.EncodeToString(hash[:])
	message := timestamp + "\n" + digestHex + "\n" + r.URL.Path
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(message))
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(provided)) {
		return nil, "", domain.Invalid(codePrefix+"_SIGNATURE_INVALID", subject+" ingress 签名校验失败")
	}
	return body, "sha256:" + digestHex, nil
}

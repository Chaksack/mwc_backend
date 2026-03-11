package services

import (
    "crypto/sha1"
    "encoding/hex"
    "errors"
    "fmt"
    "path"
    "strings"
    "time"

    "mwc_backend/config"
)

// StorageService provides minimal helpers for generating object keys and returning a "presigned" upload descriptor.
// NOTE: This is a lightweight implementation without AWS SDK dependencies. It constructs an unsigned POST target
// URL for the bucket which is sufficient for local/dev testing. In production, replace with real S3 presign.
type StorageService struct {
    cfg *config.Config
}

func NewStorageService(cfg *config.Config) *StorageService {
    return &StorageService{cfg: cfg}
}

// PresignedPost represents a minimal POST upload contract returned to the client
// for direct-to-storage uploads.
type PresignedPost struct {
    URL       string            `json:"url"`
    Fields    map[string]string `json:"fields"`
    Key       string            `json:"key"`
    ExpiresAt time.Time         `json:"expires_at"`
    Bucket    string            `json:"bucket"`
    Region    string            `json:"region"`
    // Max size hint for the client (not enforced server-side here)
    MaxSize int64 `json:"max_size"`
    // ContentType hint
    ContentType string `json:"content_type"`
}

// CreatePresignedUpload returns a minimal presigned POST descriptor for uploading a single object.
// This implementation does NOT generate actual AWS signatures; it is suitable for MVP/dev
// and environments where uploads are proxied/validated elsewhere.
func (s *StorageService) CreatePresignedUpload(key string, contentType string, maxSize int64) (*PresignedPost, error) {
    if strings.TrimSpace(key) == "" {
        return nil, errors.New("key is required")
    }
    // Production hardening: refuse returning unsigned presign in prod unless explicitly allowed by config
    env := strings.ToLower(strings.TrimSpace(s.cfg.Environment))
    if (env == "prod" || env == "production") && !s.cfg.AllowUnsignedPresign {
        return nil, errors.New("unsigned presign is disabled in production; please enable AWS SigV4 presign")
    }
    bucket := strings.TrimSpace(s.cfg.S3Bucket)
    region := strings.TrimSpace(s.cfg.S3Region)
    if bucket == "" {
        return nil, errors.New("S3 bucket not configured")
    }
    if region == "" {
        region = "us-east-1"
    }
    url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com", bucket, region)
    expires := time.Now().Add(15 * time.Minute)
    fields := map[string]string{
        "key":         key,
        "Content-Type": contentType,
    }
    return &PresignedPost{
        URL:         url,
        Fields:      fields,
        Key:         key,
        ExpiresAt:   expires,
        Bucket:      bucket,
        Region:      region,
        MaxSize:     maxSize,
        ContentType: contentType,
    }, nil
}

// GenerateResumeObjectKey generates a namespaced object key for a resume file using a datestamped prefix
// and a short hash of the original filename to reduce collisions.
func GenerateResumeObjectKey(fileName string) string {
    name := path.Base(strings.TrimSpace(fileName))
    if name == "" {
        name = "resume.pdf"
    }
    // Hash the filename with timestamp to avoid very long keys and reduce PII in object names
    h := sha1.New()
    ts := time.Now().UTC().Format(time.RFC3339Nano)
    _, _ = h.Write([]byte(ts + "::" + name))
    digest := hex.EncodeToString(h.Sum(nil))[:16]
    y, m, _ := time.Now().Date()
    prefix := fmt.Sprintf("resumes/%04d-%02d", y, int(m))
    ext := ""
    if idx := strings.LastIndex(name, "."); idx > -1 {
        ext = name[idx:]
    }
    return fmt.Sprintf("%s/%s%s", prefix, digest, ext)
}

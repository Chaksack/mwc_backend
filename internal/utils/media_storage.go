package utils

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type UploadedMedia struct {
	URL       string
	Storage   string
	ObjectKey string
	FileName  string
}

type s3State struct {
	enabled       bool
	initErr       error
	bucket        string
	region        string
	prefix        string
	publicBaseURL string
	presignTTL    time.Duration

	client   *s3.Client
	presign  *s3.PresignClient
	initOnce sync.Once
}

var globalS3 s3State

func getEnvAny(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func (s *s3State) init(ctx context.Context) {
	s.initOnce.Do(func() {
		bucket := strings.TrimSpace(os.Getenv("S3_BUCKET"))
		if bucket == "" {
			s.enabled = false
			return
		}

		region := getEnvAny("AWS_REGION", "AWS_DEFAULT_REGION")
		if region == "" {
			s.initErr = fmt.Errorf("S3_BUCKET is set but AWS_REGION/AWS_DEFAULT_REGION is missing")
			return
		}

		prefix := strings.Trim(strings.TrimSpace(os.Getenv("S3_PREFIX")), "/")
		if prefix == "" {
			prefix = "uploads"
		}

		publicBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("S3_PUBLIC_BASE_URL")), "/")
		if publicBaseURL != "" && !strings.HasPrefix(publicBaseURL, "http://") && !strings.HasPrefix(publicBaseURL, "https://") {
			publicBaseURL = "https://" + publicBaseURL
		}

		presignTTLSeconds := 3600
		if v := strings.TrimSpace(os.Getenv("S3_PRESIGN_TTL_SECONDS")); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				presignTTLSeconds = parsed
			}
		}

		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
		if err != nil {
			s.initErr = fmt.Errorf("failed to load AWS config: %w", err)
			return
		}

		client := s3.NewFromConfig(awsCfg)
		s.enabled = true
		s.bucket = bucket
		s.region = region
		s.prefix = prefix
		s.publicBaseURL = publicBaseURL
		s.presignTTL = time.Duration(presignTTLSeconds) * time.Second
		s.client = client
		s.presign = s3.NewPresignClient(client)
	})
}

func S3Enabled(ctx context.Context) (bool, error) {
	globalS3.init(ctx)
	return globalS3.enabled, globalS3.initErr
}

func sanitizeOriginalName(original string) (safeOriginal string, ext string) {
	safeOriginal = filepath.Base(original)
	reUnsafe := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	safeOriginal = reUnsafe.ReplaceAllString(safeOriginal, "_")
	if safeOriginal == "" {
		safeOriginal = "upload"
	}
	ext = filepath.Ext(safeOriginal)
	return safeOriginal, ext
}

func buildS3Key(category string, storedName string) (string, error) {
	cat := strings.Trim(category, "/")
	if cat == "" {
		return "", fmt.Errorf("category is required")
	}
	key := path.Join(globalS3.prefix, cat, storedName)
	return key, nil
}

func buildPersistedS3URL(key string) string {
	if globalS3.publicBaseURL != "" {
		// Support either:
		//  - S3_PUBLIC_BASE_URL=https://<cdn-domain>              (most common)
		//  - S3_PUBLIC_BASE_URL=https://<cdn-domain>/<S3_PREFIX>  (when origin path is configured)
		// Avoid generating URLs like /uploads/uploads/... in the second case.
		prefix := strings.Trim(globalS3.prefix, "/")
		base := strings.TrimRight(globalS3.publicBaseURL, "/")
		keyClean := strings.TrimLeft(key, "/")
		if prefix != "" && strings.HasSuffix(base, "/"+prefix) {
			keyClean = strings.TrimPrefix(keyClean, prefix+"/")
		}
		return base + "/" + keyClean
	}
	return "s3://" + globalS3.bucket + "/" + key
}

func PutObjectToS3(ctx context.Context, key string, contentType string, body io.Reader) error {
	globalS3.init(ctx)
	if !globalS3.enabled {
		return fmt.Errorf("S3 is not enabled")
	}
	if globalS3.initErr != nil {
		return globalS3.initErr
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := globalS3.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(globalS3.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	return err
}

func DeleteObjectFromS3(ctx context.Context, key string) error {
	globalS3.init(ctx)
	if !globalS3.enabled {
		return fmt.Errorf("S3 is not enabled")
	}
	if globalS3.initErr != nil {
		return globalS3.initErr
	}
	_, err := globalS3.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(globalS3.bucket),
		Key:    aws.String(key),
	})
	return err
}

func ResolveS3URL(ctx context.Context, key string) (string, error) {
	globalS3.init(ctx)
	if !globalS3.enabled {
		return "", fmt.Errorf("S3 is not enabled")
	}
	if globalS3.initErr != nil {
		return "", globalS3.initErr
	}

	if globalS3.publicBaseURL != "" {
		prefix := strings.Trim(globalS3.prefix, "/")
		base := strings.TrimRight(globalS3.publicBaseURL, "/")
		keyClean := strings.TrimLeft(key, "/")
		if prefix != "" && strings.HasSuffix(base, "/"+prefix) {
			keyClean = strings.TrimPrefix(keyClean, prefix+"/")
		}
		return base + "/" + keyClean, nil
	}

	out, err := globalS3.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(globalS3.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = globalS3.presignTTL
	})
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func parseS3URI(raw string) (bucket string, key string, ok bool) {
	if !strings.HasPrefix(raw, "s3://") {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	bucket = u.Host
	key = strings.TrimPrefix(u.Path, "/")
	if bucket == "" || key == "" {
		return "", "", false
	}
	return bucket, key, true
}

// ResolveMediaURL returns an https URL for a stored media reference.
//   - If rawURL is already http(s), it is returned unchanged.
//   - If storage is "s3" and objectKey is set, it returns either the public base URL (if configured)
//     or a presigned GET URL.
//   - If rawURL is an s3:// URI, it will be resolved similarly.
//
// Best-effort: on errors, it returns the input rawURL.
func ResolveMediaURL(ctx context.Context, rawURL string, storage string, objectKey string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}

	if strings.EqualFold(storage, "s3") {
		key := strings.TrimSpace(objectKey)
		if key == "" {
			_, parsedKey, ok := parseS3URI(rawURL)
			if ok {
				key = parsedKey
			}
		}
		if key != "" {
			resolved, err := ResolveS3URL(ctx, key)
			if err == nil && resolved != "" {
				return resolved
			}
		}
	}

	// If it looks like s3://bucket/key but storage not set, still try.
	_, key, ok := parseS3URI(rawURL)
	if ok {
		resolved, err := ResolveS3URL(ctx, key)
		if err == nil && resolved != "" {
			return resolved
		}
	}

	return rawURL
}

// SaveUploadedFile stores an uploaded file either in S3 (if S3_BUCKET is set) or on disk.
// It returns a persisted URL (either https public URL or s3://... for S3; or /uploads/... for disk)
// plus metadata needed to later resolve an https URL for clients.
func SaveUploadedFile(ctx context.Context, originalFilename string, contentType string, body io.Reader, localDir string, localURLPrefix string, s3Category string, ownerID uint) (UploadedMedia, string, error) {
	safeOriginal, ext := sanitizeOriginalName(originalFilename)
	storedName := fmt.Sprintf("%d_%d%s", ownerID, time.Now().UnixNano(), ext)

	// Prefer provided content-type; fall back to extension mapping.
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}

	enabled, err := S3Enabled(ctx)
	if err != nil {
		return UploadedMedia{}, "", err
	}

	if enabled {
		key, err := buildS3Key(s3Category, storedName)
		if err != nil {
			return UploadedMedia{}, "", err
		}
		if err := PutObjectToS3(ctx, key, contentType, body); err != nil {
			return UploadedMedia{}, "", err
		}
		persisted := buildPersistedS3URL(key)
		return UploadedMedia{URL: persisted, Storage: "s3", ObjectKey: key, FileName: safeOriginal}, storedName, nil
	}

	// Disk fallback
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return UploadedMedia{}, "", err
	}
	// Write to disk
	filePath := filepath.Join(localDir, storedName)
	f, err := os.Create(filePath)
	if err != nil {
		return UploadedMedia{}, "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return UploadedMedia{}, "", err
	}

	localURLPrefix = "/" + strings.Trim(localURLPrefix, "/")
	persisted := localURLPrefix + "/" + storedName
	return UploadedMedia{URL: persisted, Storage: "local", ObjectKey: "", FileName: safeOriginal}, storedName, nil
}

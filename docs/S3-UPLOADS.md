# S3 Uploads (Media Storage)

This backend supports storing uploaded media (profile pictures, institution profile pictures, resumes) in AWS S3.

## Behavior

- If `S3_BUCKET` is **not** set, uploads are written to the local filesystem under `./uploads/*` and served via Fiber static routes:
  - `/uploads/*`
  - `/api/v1/uploads/*` (compatibility)

- If `S3_BUCKET` **is** set, uploads are stored in S3.
  - If `S3_PUBLIC_BASE_URL` is set, persisted URLs can be stable public URLs.
  - Otherwise, persisted values may be stored as `s3://<bucket>/<key>` and API responses will resolve them to **presigned GET URLs** at runtime.

## Environment variables

Required:
- `S3_BUCKET`
- `AWS_REGION` (or `AWS_DEFAULT_REGION`)

Optional:
- `S3_PREFIX` (default: `uploads`)
- `S3_PUBLIC_BASE_URL` (recommended in production for stable URLs, e.g. CloudFront). Typically this is your CloudFront distribution root (e.g. `https://dxxxx.cloudfront.net`). If your CloudFront origin path is set to `/<S3_PREFIX>`, this backend also supports setting `S3_PUBLIC_BASE_URL` to include the prefix.
- `S3_PRESIGN_TTL_SECONDS` (default: `3600`)

## Notes

- AWS credentials are provided via the standard AWS SDK v2 credential chain (environment variables, shared config, or instance/task role).
- Deleting a profile picture attempts a best-effort delete from S3 when the stored record indicates S3-backed storage.

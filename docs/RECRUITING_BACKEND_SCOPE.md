# Recruiting Backend: Scope and Timeline

This document scopes the backend work required to deliver the recruiting features described previously (public careers site + admin/HR workflow) based on the current MWC backend codebase. It identifies what already exists, what’s missing, and a pragmatic timeline with milestones.

## Current backend capabilities to leverage (from repo)
- Stack: Go + Fiber, GORM (Postgres), RabbitMQ, SMTP email, Swagger, Dockerized deploy, health/metrics.
- Auth: JWT-based auth with roles in `models.UserRole` (Parent, Professional, Institution, Admin). Middleware in `internal/api/middleware/auth.go`.
- Models (relevant):
  - `Job` and `JobApplication` exist in `internal/models/models.go` with links to `InstitutionProfile` and `MontessoriProfessionalProfile`.
  - `ActionLog` available for audit trails.
  - `DynamicSubscriptionPlan`, Stripe integration present but not needed for applicant side.
- Routes and handlers:
  - Jobs list endpoint exists but currently gated behind auth + subscription: `GET /api/v1/jobs` → `institutionHandler.GetAllJobs`.
  - No public careers endpoints (anonymous) and no HR/Admin recruiting endpoints yet.
  - Email service in `internal/email/gomailer.go` and background task patterns via `internal/queue`.
- Monitoring and ops: Health endpoints, Prometheus, CI to AWS ECR.

## Key gaps for recruiting
1. Public, anonymous careers endpoints and application intake.
2. Admin/HR endpoints for job CRUD, publishing, and applicant pipeline management.
3. File handling for resumes (S3 presigned upload or server-side upload + storage URL).
4. Captcha verification for application submission (optional but recommended).
5. Email notifications and templates for applicant and HR communications.
6. Expanded data model for publishing controls, per-job custom questions (optional), application status events/audit, and applicant contact info when not a registered user.
7. Swagger docs for all new models and endpoints.
8. RBAC guards for HR roles (recruiter, hiring_manager, admin) and route groups.
9. Basic reporting and CSV export endpoints.

## Proposed backend scope (delta from current code)

### 1) Data model additions/adjustments (GORM)
- Job enhancements (non-breaking, additive):
  - Add `Slug string 'gorm:"uniqueIndex"'`, `Department string`, `IsPublished bool` (separate from IsActive), `PublishedAt *time.Time`.
  - Optional: `CustomQuestions JSONB` (array of question objects) for MVP-lite instead of separate table.
- Public application intake:
  - Extend `JobApplication` to support public applicants (not only MontessoriProfessional): add fields
    - `ApplicantName string`, `ApplicantEmail string`, `ApplicantPhone string`, `CoverLetter string`, `ResumeURL string`, `Source string`, `Consent bool`.
    - Keep existing foreign key to `MontessoriProfessionalProfileID` nullable for registered applicants.
- Application events:
  - New `ApplicationEvent` model: `ApplicationID uint`, `ActorID *uint`, `Action string`, `Notes string`, `CreatedAt time`.

Notes:
- All changes are backward-compatible (additive). Update `AutoMigrate` to include new model and columns.

### 2) Configuration
- Add to `config.Config` and env support:
  - `S3Bucket`, `S3Region`, `S3AccessKey`, `S3SecretKey` or prefer IAM/role in runtime; for local dev, env vars.
  - `ResumeMaxSizeMB` (e.g., 10), `ResumeAllowedTypes` ("application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document").
  - `CaptchaProvider` (none|recaptcha|hcaptcha), `CaptchaSecret`.

### 3) Services
- File upload service for presigned URLs (S3): `internal/services/storage_service.go` with methods:
  - `CreatePresignedUpload(key string, contentType string, maxSize int64) (url, fields, expiresAt)`.
- Captcha verification service: `internal/services/captcha_service.go` with `Verify(ctx, token string) (bool, error)`.
- Recruiting notification service functions using existing email service (templated): applicant receipt, HR new-application, interview invite.

### 4) Public API endpoints (no auth)
- `GET /api/v1/careers/jobs` — list published jobs with filters (location, department, type, q, page, page_size, sort).
- `GET /api/v1/careers/jobs/:slug` — job detail by slug.
- `POST /api/v1/careers/resume/presign` — return S3 presigned POST for resume upload (rate limited, captcha optional).
- `POST /api/v1/careers/jobs/:id/apply` — application intake (validates captcha if configured, accepts JSON with fields + uploaded ResumeURL).

### 5) Admin/HR API endpoints (JWT + role guard)
- Jobs management:
  - `POST /api/v1/admin/recruiting/jobs` — create (draft by default).
  - `PUT /api/v1/admin/recruiting/jobs/:id` — update.
  - `PATCH /api/v1/admin/recruiting/jobs/:id/publish` — publish/unpublish.
  - `GET /api/v1/admin/recruiting/jobs` — list (all, filters, pagination).
- Applications management:
  - `GET /api/v1/admin/recruiting/applications` — list/filter.
  - `GET /api/v1/admin/recruiting/applications/:id` — detail (include job, events).
  - `PATCH /api/v1/admin/recruiting/applications/:id/status` — change status, record event, optional email.
  - `POST /api/v1/admin/recruiting/applications/:id/email` — send templated email (interview invite, rejection, custom).
- Export & reports:
  - `GET /api/v1/admin/recruiting/reports/overview` — summary metrics.
  - `GET /api/v1/admin/recruiting/export.csv` — CSV export of applications (filtered).

RBAC: authorize `admin`, `recruiter`, `hiring_manager` roles. Add these roles to `models.UserRole` and ensure middleware checks them. For now, map to existing `Admin` until UI/seed is ready.

### 6) Handlers and routing
- Create `internal/api/handlers/recruiting_public_handler.go` for public endpoints.
- Create `internal/api/handlers/recruiting_admin_handler.go` for admin endpoints.
- Wire routes in `internal/api/routes.go` under groups:
  - `apiV1.Group("/careers")` for public.
  - `apiV1.Group("/admin/recruiting", authMw)` with role guard middleware.

### 7) Swagger documentation
- Update `docs/swagger.yaml` and `docs/swagger.json` to include:
  - Schemas: `JobPublic`, `JobDetail`, `ApplicationCreate`, `Application`, `ApplicationEvent`.
  - Paths for all endpoints above with parameters, responses, and examples.

### 8) Email templates
- Add recruiting templates in `internal/email/templates/` (string constants if avoiding file I/O):
  - `application_received`, `interview_invite`, `rejection`, `status_update`.

### 9) Metrics and logging
- Counters: applications submitted per job and per source.
- Histogram: application processing duration (optional).
- Audit: use `ActionLog` for job publish/unpublish and application status changes.

### 10) Rate limiting and validation
- Apply per-IP rate limit on resume presign and apply endpoints (Fiber middleware or lightweight custom store).
- Server-side validation: allowed file types, sizes, required applicant fields, consent flag.

## Work breakdown by repo touchpoints
- `internal/models/models.go`: extend `Job`, `JobApplication`; add `ApplicationEvent`; update `AutoMigrate`.
- `config/config.go`: add S3 and captcha-related config.
- `internal/services/`: add `storage_service.go`, `captcha_service.go`, extend `notification_service.go` with recruiting methods.
- `internal/api/handlers/`: add recruiting handlers; reuse existing patterns for responses and errors.
- `internal/api/routes.go`: register route groups.
- `internal/email/`: add recruiting templates; possibly small helper methods.
- `internal/metrics/metrics.go`: add counters and expose via existing endpoints.
- `docs/swagger.yaml` + `docs/swagger.json`: add schemas and paths; regenerate `docs/docs.go` if applicable.
- Tests: add `tests/recruiting_public_apply.go` and `tests/recruiting_admin.go` smoke tests (optional in this phase).

## Timeline and milestones (single squad; 2–3 backend engineers + QA)

Duration: ~6–8 weeks for MVP completeness. An accelerated 4–5 week track is possible by deferring captcha, exports, and some admin niceties.

Milestones
1) Week 0 – Readiness and decisions
   - Confirm role names/permissions, file storage choice (S3 bucket), captcha provider, email sender identities.
   - Provision S3 bucket and credentials; set env vars in dev/staging. Define allowed resume types/sizes.

2) Weeks 1–2 – Foundations and data model ✓
   - Implement model changes and auto-migrations; seed roles if needed.
   - Implement storage and captcha services; extend config.
   - Scaffold handlers and routes (no business logic yet); add Swagger stubs.
   - CI/env updates as needed; health checks extended to validate S3/captcha config.

3) Weeks 3–4 – Public careers MVP
   - Implement `GET /careers/jobs`, `GET /careers/jobs/:slug` with filters.
   - Implement presigned resume upload and `POST /careers/jobs/:id/apply` with validation, captcha, and emails (applicant receipt + HR notify).
   - Add metrics and basic rate limiting; update Swagger and examples.

4) Weeks 5–6 – Admin/HR MVP
   - Implement job CRUD + publish/unpublish; list/filter jobs.
   - Applications list/detail; status changes with event log and templated emails.
   - RBAC enforcement; ActionLog entries; CSV export.

5) Week 7 – Reporting & hardening
   - Overview metrics; performance tuning of queries and indexes.
   - Error handling and a11y/support for email templates; finalize audit trails.

6) Week 8 – QA, UAT, launch
   - End-to-end test passes for apply flow; load test application endpoint; security and privacy review.
   - Staging sign-off; production rollout via feature flag (hide public endpoints by config until go-live).

## Acceptance criteria (backend)
- Public endpoints serve only published jobs and support filters + pagination.
- Application submission validates required fields, captcha (if enabled), and file constraints; data persisted with events.
- HR endpoints permit authorized users to manage jobs and applications; actions audited; emails sent where applicable.
- Swagger documents all new endpoints and models; health check reflects configured services; metrics expose application counts.

## Risks and mitigations
- File security: enforce content type/size; optionally add AV scan hook later.
- Spam/bots: captcha + rate limiting.
- PII handling: store minimum required; add retention policy job (later iteration).
- Role provisioning: map to existing Admin initially; add dedicated HR roles in a subsequent seed/migration.

## Out of scope (post-MVP)
- Job board feeds (Indeed/LinkedIn XML), calendar integrations, advanced search, referral tracking.

---

## CODE_REVIEW_2026-03-04

This section captures a comprehensive code review of the backend (Fiber + GORM), including the newly introduced recruiting features. It summarizes strengths, risks, and prioritized recommendations. For full context, see the repository files referenced below.

### Strengths
- Clear modular structure across config, handlers, services, models, middleware, metrics, and store.
- Recruiting features integrated thoughtfully: public careers endpoints, admin management, institution publish flow, captcha hooks, S3 key generation, rate limiting, basic metrics, and smoke tests.
- Database models are additive and coherent; indexes on critical fields; AutoMigrate includes all models.
- Health and monitoring endpoints exist; Prometheus export is wired.
- Email service includes no-op fallback and address normalization to ease dev.

### Critical and High Risks
1) Unsigned storage presign
   - File: internal/services/storage_service.go
   - Issue: CreatePresignedUpload returns an unsigned POST target. Risk of failure in prod or abuse if bucket policy is permissive.
   - Action: Implement AWS SigV4 presign (SDK v2) or proxy uploads; enforce content-length-range and SSE. Gate by env flag in prod.

2) Swagger drift (multiple sources)
   - Files: docs/swagger.yaml, docs/swagger.json, docs/docs.go, internal/api/swagger.go
   - Issue: Versions drift (2.1.8 vs 2.2.0); JSON not regenerated; YAML + annotations mix.
   - Action: Pick single source (recommended: Go annotations + swag init). Add CI to validate and fail on drift.

3) Background scheduler lifecycle
   - File: internal/api/routes.go
   - Issue: schedulerService.Start launched in routes; risks multiple starts and no graceful shutdown.
   - Action: Start in main.go with context + signal handling; ensure idempotent Start/Stop.

4) Authorization and ownership checks
   - Ensure consistent ownership validation across institution update/delete applicant endpoints; add positive/negative tests.

### Medium Risks and Improvements
- Input validation: introduce go-playground/validator; centralize error shapes.
- Public apply: keep email regex lightweight; require captcha in prod; optionally normalize phone.
- In-memory metrics: acceptable for MVP; document limitations or adopt Prometheus client counters.
- Professional ApplyForJob: align resume handling with public S3 presign and validations.
- Health completeness: ensure DB failures downgrade health.
- Logging/PII: standardize structure and avoid sensitive PII; add request IDs.

### Low Risks / DX
- Dependency hygiene: ensure go mod tidy/build in CI; pin limiter version.
- Add golangci-lint; make targets for build/test/lint/swag.
- Extend audit logs for admin recruiting actions (in addition to ApplicationEvent).

### API and Routing Notes
- Public careers grouped under /careers; admin under /admin/recruiting; institution under /institution; auth middleware order fixed.
- Rate limiting on write endpoints only; consider Redis store for multi-replica.

### Quick Wins (1–2 days)
- Decide Swagger source of truth; bump versions consistently (2.2.x); add CI check.
- Add golangci-lint action and minimal config.
- Move scheduler Start to main.go; add graceful shutdown.
- In prod, require CAPTCHA_PROVIDER != none.

### Near-term (1–2 weeks)
- Implement real S3 SigV4 presign (POST/PUT) with policy and tests.
- Consolidate Swagger generation on CI; document in README.
- Introduce ApplicationService to centralize apply logic and notifications.
- Add tests for authorization/ownership and validations.

### References
- Fiber: https://docs.gofiber.io
- GORM: https://gorm.io
- golangci-lint: https://github.com/golangci/golangci-lint and https://github.com/golangci/golangci-lint-action

### Conclusion
The codebase is well-structured and the recruiting functionality is thoughtfully integrated. Addressing presign security, Swagger consolidation, production safeguards (captcha, scheduler lifecycle), and CI lint/tests will significantly improve reliability and developer velocity.

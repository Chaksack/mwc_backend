package services

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "net/http"
    "strings"
    "time"

    "mwc_backend/config"
)

// CaptchaService provides verification for captcha providers. If provider is "none" or empty,
// Verify is a no-op that returns true.
type CaptchaService struct {
    provider string
    secret   string
    client   *http.Client
}

func NewCaptchaService(cfg *config.Config) *CaptchaService {
    provider := strings.ToLower(strings.TrimSpace(cfg.CaptchaProvider))
    secret := strings.TrimSpace(cfg.CaptchaSecret)
    return &CaptchaService{
        provider: provider,
        secret:   secret,
        client: &http.Client{Timeout: 4 * time.Second},
    }
}

// Verify validates the captcha token with the configured provider.
// Returns true on success, or true as a no-op when provider is none/empty.
func (s *CaptchaService) Verify(ctx context.Context, token string) (bool, error) {
    if s.provider == "" || s.provider == "none" {
        return true, nil
    }
    t := strings.TrimSpace(token)
    if t == "" {
        return false, errors.New("captcha token is required")
    }
    if s.secret == "" {
        return false, errors.New("captcha secret is not configured")
    }

    switch s.provider {
    case "hcaptcha":
        return s.verifyHCaptcha(ctx, t)
    case "recaptcha", "recaptcha_v2", "recaptcha_v3":
        return s.verifyReCaptcha(ctx, t)
    default:
        // Unknown provider -> treat as disabled (no-op) to avoid hard failing deployments
        return true, nil
    }
}

func (s *CaptchaService) verifyHCaptcha(ctx context.Context, token string) (bool, error) {
    endpoint := "https://hcaptcha.com/siteverify"
    body := "secret=" + s.secret + "&response=" + token
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    resp, err := s.client.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()
    var out struct{ Success bool `json:"success"` }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return false, err
    }
    return out.Success, nil
}

func (s *CaptchaService) verifyReCaptcha(ctx context.Context, token string) (bool, error) {
    endpoint := "https://www.google.com/recaptcha/api/siteverify"
    body := "secret=" + s.secret + "&response=" + token
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    resp, err := s.client.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()
    var out struct{ Success bool `json:"success"` }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return false, err
    }
    return out.Success, nil
}

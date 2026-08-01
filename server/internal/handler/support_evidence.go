package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/pkg/redact"
)

const (
	supportEvidenceResponseMaxBytes  = 256 * 1024
	supportEvidenceSourceMaxRunes    = 8 * 1024
	supportEvidenceTotalMaxRunes     = 48 * 1024
	supportEvidenceReferenceMaxRunes = 2048
	supportEvidenceMaxSources        = 24
	supportEvidenceMaxLimitations    = 12
)

type supportEvidenceProvider interface {
	Collect(context.Context, supportEvidenceRequest) (supportEvidenceBundle, error)
}

type supportEvidenceRequest struct {
	CaseID         string                 `json:"case_id"`
	WorkspaceID    string                 `json:"workspace_id"`
	ReporterUserID string                 `json:"reporter_user_id"`
	ReporterEmail  string                 `json:"reporter_email"`
	ReporterName   string                 `json:"reporter_name"`
	ApplicationKey string                 `json:"application_key"`
	UnitID         string                 `json:"unit_id,omitempty"`
	Conversation   []supportPromptMessage `json:"conversation"`
}

type supportEvidenceSource struct {
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	Reference        string `json:"reference"`
	Content          string `json:"content"`
	ObservedAt       string `json:"observed_at,omitempty"`
	DeployedRevision string `json:"deployed_revision,omitempty"`
}

type supportEvidenceBundle struct {
	Sources     []supportEvidenceSource `json:"sources"`
	Limitations []string                `json:"limitations,omitempty"`
}

type httpSupportEvidenceClient struct {
	endpoint string
	token    string
	client   *http.Client
}

func newHTTPSupportEvidenceClient(endpoint, token string, timeout time.Duration) (*httpSupportEvidenceClient, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse support evidence endpoint: %w", err)
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.Host == "" {
		return nil, errors.New("support evidence endpoint must be an absolute URL without user info or fragment")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackEvidenceHost(parsed.Hostname())) {
		return nil, errors.New("support evidence endpoint must use HTTPS unless it targets loopback")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &httpSupportEvidenceClient{
		endpoint: parsed.String(),
		token:    strings.TrimSpace(token),
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func isLoopbackEvidenceHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *httpSupportEvidenceClient) Collect(ctx context.Context, request supportEvidenceRequest) (supportEvidenceBundle, error) {
	if c == nil || c.client == nil || c.endpoint == "" {
		return supportEvidenceBundle{}, errors.New("support evidence provider is disabled")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return supportEvidenceBundle{}, fmt.Errorf("encode support evidence request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return supportEvidenceBundle{}, fmt.Errorf("create support evidence request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	if c.token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.client.Do(httpRequest)
	if err != nil {
		return supportEvidenceBundle{}, fmt.Errorf("collect support evidence: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return supportEvidenceBundle{}, fmt.Errorf("support evidence provider returned status %d", response.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, supportEvidenceResponseMaxBytes+1))
	if err != nil {
		return supportEvidenceBundle{}, fmt.Errorf("read support evidence response: %w", err)
	}
	if len(raw) > supportEvidenceResponseMaxBytes {
		return supportEvidenceBundle{}, errors.New("support evidence response exceeds size limit")
	}
	var bundle supportEvidenceBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return supportEvidenceBundle{}, fmt.Errorf("decode support evidence response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return supportEvidenceBundle{}, errors.New("support evidence response must contain exactly one JSON object")
	}
	return normalizeSupportEvidence(bundle)
}

func normalizeSupportEvidence(bundle supportEvidenceBundle) (supportEvidenceBundle, error) {
	if len(bundle.Sources) > supportEvidenceMaxSources {
		return supportEvidenceBundle{}, errors.New("support evidence response contains too many sources")
	}
	if len(bundle.Limitations) > supportEvidenceMaxLimitations {
		return supportEvidenceBundle{}, errors.New("support evidence response contains too many limitations")
	}

	normalized := supportEvidenceBundle{
		Sources:     make([]supportEvidenceSource, 0, len(bundle.Sources)),
		Limitations: make([]string, 0, len(bundle.Limitations)),
	}
	remaining := supportEvidenceTotalMaxRunes
	for _, source := range bundle.Sources {
		source.Kind = strings.ToLower(strings.TrimSpace(source.Kind))
		if !oneOf(source.Kind, "knowledge", "code", "data") {
			return supportEvidenceBundle{}, fmt.Errorf("unsupported support evidence kind %q", source.Kind)
		}
		source.Title = truncateSupportText(redact.Text(strings.TrimSpace(source.Title)), supportEvidenceReferenceMaxRunes)
		source.Reference = truncateSupportText(redact.Text(strings.TrimSpace(source.Reference)), supportEvidenceReferenceMaxRunes)
		source.ObservedAt = truncateSupportText(redact.Text(strings.TrimSpace(source.ObservedAt)), 128)
		source.DeployedRevision = truncateSupportText(redact.Text(strings.TrimSpace(source.DeployedRevision)), 256)
		if source.Reference == "" {
			return supportEvidenceBundle{}, errors.New("support evidence source is missing its reference")
		}
		if source.ObservedAt != "" {
			if _, err := time.Parse(time.RFC3339, source.ObservedAt); err != nil {
				return supportEvidenceBundle{}, errors.New("support evidence source has an invalid observed_at timestamp")
			}
		}
		if source.Kind == "code" && source.DeployedRevision == "" {
			return supportEvidenceBundle{}, errors.New("code evidence is missing its deployed revision")
		}
		if source.Kind == "data" && source.ObservedAt == "" {
			return supportEvidenceBundle{}, errors.New("data evidence is missing its observation timestamp")
		}
		maxRunes := supportEvidenceSourceMaxRunes
		if remaining < maxRunes {
			maxRunes = remaining
		}
		source.Content = truncateSupportText(redact.Text(strings.TrimSpace(source.Content)), maxRunes)
		if source.Content == "" {
			return supportEvidenceBundle{}, errors.New("support evidence source is missing its content")
		}
		remaining -= len([]rune(source.Content))
		normalized.Sources = append(normalized.Sources, source)
		if remaining == 0 {
			break
		}
	}
	for _, limitation := range bundle.Limitations {
		limitation = truncateSupportText(redact.Text(strings.TrimSpace(limitation)), 1024)
		if limitation != "" {
			normalized.Limitations = append(normalized.Limitations, limitation)
		}
	}
	return normalized, nil
}

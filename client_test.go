package mailx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendEmailSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/emails" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing auth header")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Email{ID: "em_1", Status: "queued"})
	}))
	defer srv.Close()

	c := NewClient("test-key", WithBaseURL(srv.URL))
	email, err := c.SendEmail(context.Background(), SendEmailRequest{From: "a@b.com", To: []string{"c@d.com"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email.ID != "em_1" {
		t.Fatalf("unexpected id: %s", email.ID)
	}
}

func TestRetriesOn429ThenSucceeds(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "rate_limited", "code": "too_many_requests", "message": "slow down"}})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Email{ID: "em_2"})
	}))
	defer srv.Close()

	c := NewClient("test-key", WithBaseURL(srv.URL))
	email, err := c.SendEmail(context.Background(), SendEmailRequest{From: "a@b.com", To: []string{"c@d.com"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if email.ID != "em_2" {
		t.Fatalf("unexpected id: %s", email.ID)
	}
}

func TestNonRetryableErrorReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "invalid_request", "code": "missing_field", "message": "from is required"}})
	}))
	defer srv.Close()

	c := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := c.SendEmail(context.Background(), SendEmailRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Status != 400 || apiErr.Code != "missing_field" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestGetEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/emails/em_3" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Email{ID: "em_3", Status: "delivered"})
	}))
	defer srv.Close()

	c := NewClient("test-key", WithBaseURL(srv.URL))
	email, err := c.GetEmail(context.Background(), "em_3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email.Status != "delivered" {
		t.Fatalf("unexpected status: %s", email.Status)
	}
}

// TestResourceMethodsHitTheExpectedRouteAndMethod is a table-driven check
// that every generic resource()-backed method (the ones that just proxy
// JSON straight through) calls the exact HTTP method and path the API
// documents - the thing most likely to silently drift or typo, since none
// of these have a typed response to catch a shape mismatch at compile time.
func TestResourceMethodsHitTheExpectedRouteAndMethod(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()
	c := NewClient("test-key", WithBaseURL(srv.URL))
	ctx := context.Background()

	cases := []struct {
		name         string
		call         func() error
		method, path string
	}{
		{"Whoami", func() error { _, err := c.Whoami(ctx); return err }, "GET", "/v1/whoami"},
		{"VerifyDomain", func() error { _, err := c.VerifyDomain(ctx, "d1"); return err }, "POST", "/v1/domains/d1/verify"},
		{"GetDKIM", func() error { _, err := c.GetDKIM(ctx, "d1"); return err }, "GET", "/v1/domains/d1/dkim"},
		{"CreateDKIM", func() error { _, err := c.CreateDKIM(ctx, "d1"); return err }, "POST", "/v1/domains/d1/dkim"},
		{"VerifySPF", func() error { _, err := c.VerifySPF(ctx, "d1"); return err }, "POST", "/v1/domains/d1/spf/verify"},
		{"VerifyDMARC", func() error { _, err := c.VerifyDMARC(ctx, "d1"); return err }, "POST", "/v1/domains/d1/dmarc/verify"},
		{"PreviewTemplate", func() error { _, err := c.PreviewTemplate(ctx, "t1", JSON{}); return err }, "POST", "/v1/templates/t1/preview"},
		{"GetEmailEvents", func() error { _, err := c.GetEmailEvents(ctx, "em1"); return err }, "GET", "/v1/emails/em1/events"},
		{"UpdateAudience", func() error { _, err := c.UpdateAudience(ctx, "a1", JSON{}); return err }, "PATCH", "/v1/audiences/a1"},
		{"AddAudienceMember", func() error { _, err := c.AddAudienceMember(ctx, "a1", JSON{}); return err }, "POST", "/v1/audiences/a1/contacts"},
		{"ListAudienceMembers", func() error { _, err := c.ListAudienceMembers(ctx, "a1"); return err }, "GET", "/v1/audiences/a1/contacts"},
		{"RemoveAudienceMember", func() error { return c.RemoveAudienceMember(ctx, "a1", "c1") }, "DELETE", "/v1/audiences/a1/contacts/c1"},
		{"PreviewBroadcast", func() error { _, err := c.PreviewBroadcast(ctx, JSON{}); return err }, "POST", "/v1/broadcasts/preview"},
		{"ListBroadcastRecipients", func() error { _, err := c.ListBroadcastRecipients(ctx, "b1"); return err }, "GET", "/v1/broadcasts/b1/recipients"},
		{"GetSuppression", func() error { _, err := c.GetSuppression(ctx, "s1"); return err }, "GET", "/v1/suppressions/s1"},
		{"RotateWebhookSecret", func() error { _, err := c.RotateWebhookSecret(ctx, "w1"); return err }, "POST", "/v1/webhooks/w1/rotate-secret"},
		{"ListWebhookDeliveries", func() error { _, err := c.ListWebhookDeliveries(ctx, "w1"); return err }, "GET", "/v1/webhooks/w1/deliveries"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMethod != tc.method || gotPath != tc.path {
				t.Fatalf("expected %s %s, got %s %s", tc.method, tc.path, gotMethod, gotPath)
			}
		})
	}
}

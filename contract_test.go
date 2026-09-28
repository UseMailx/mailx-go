package mailx

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestContractSendEmail(t *testing.T) {
	baseURL := os.Getenv("MAILX_SDK_TEST_BASE_URL")
	apiKey := os.Getenv("MAILX_SDK_TEST_API_KEY")
	if baseURL == "" || apiKey == "" {
		t.Skip("MAILX_SDK_TEST_BASE_URL/MAILX_SDK_TEST_API_KEY not set, skipping live contract test")
	}
	c := NewClient(apiKey, WithBaseURL(baseURL))
	_, err := c.SendEmail(context.Background(), SendEmailRequest{
		From:    "sdk-test@example.com",
		To:      []string{"sdk-test-dest@example.com"},
		Subject: "Go SDK contract test",
		Text:    "hello",
	})
	if err != nil {
		t.Fatalf("live send failed: %v", err)
	}
}

// TestContractNewMethods exercises every method added in this pass against
// a real running server, not just httptest doubles - proving the request
// shapes this SDK sends are ones the real API actually accepts.
func TestContractNewMethods(t *testing.T) {
	baseURL := os.Getenv("MAILX_SDK_TEST_BASE_URL")
	apiKey := os.Getenv("MAILX_SDK_TEST_API_KEY")
	if baseURL == "" || apiKey == "" {
		t.Skip("MAILX_SDK_TEST_BASE_URL/MAILX_SDK_TEST_API_KEY not set, skipping live contract test")
	}
	c := NewClient(apiKey, WithBaseURL(baseURL))
	ctx := context.Background()

	// Suffix every created resource's name/email with a fresh run id so
	// this test is safely rerunnable against the same persistent test
	// tenant - domain/template/audience names are unique per tenant, so a
	// fixed name would conflict (domain_already_exists) on any rerun.
	run := strconv.FormatInt(time.Now().UnixNano(), 36)

	who, err := c.Whoami(ctx)
	if err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if who["organization"] == nil {
		t.Fatalf("Whoami: expected an organization, got %+v", who)
	}

	domain, err := c.CreateDomain(ctx, JSON{"name": "sdk-contract-" + run + ".example.com"})
	if err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	domainID, _ := domain["id"].(string)
	if _, err := c.VerifyDomain(ctx, domainID); err != nil {
		t.Fatalf("VerifyDomain: %v", err)
	}
	if _, err := c.GetDKIM(ctx, domainID); err != nil {
		t.Fatalf("GetDKIM: %v", err)
	}

	tmpl, err := c.CreateTemplate(ctx, JSON{"name": "sdk-contract-" + run, "subject": "Hi {{name}}", "text": "Hello {{name}}"})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	templateID, _ := tmpl["id"].(string)
	preview, err := c.PreviewTemplate(ctx, templateID, JSON{"variables": JSON{"name": "Ada"}})
	if err != nil {
		t.Fatalf("PreviewTemplate: %v", err)
	}
	if preview["subject"] != "Hi Ada" {
		t.Fatalf("PreviewTemplate: expected rendered subject, got %+v", preview)
	}

	contact, err := c.CreateContact(ctx, JSON{"email": "sdk-contract-member-" + run + "@example.com"})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	contactID, _ := contact["id"].(string)

	aud, err := c.CreateAudience(ctx, JSON{"name": "sdk-contract-audience-" + run})
	if err != nil {
		t.Fatalf("CreateAudience: %v", err)
	}
	audienceID, _ := aud["id"].(string)
	if _, err := c.UpdateAudience(ctx, audienceID, JSON{"name": "sdk-contract-audience-" + run + "-renamed"}); err != nil {
		t.Fatalf("UpdateAudience: %v", err)
	}
	if _, err := c.AddAudienceMember(ctx, audienceID, JSON{"contact_id": contactID}); err != nil {
		t.Fatalf("AddAudienceMember: %v", err)
	}
	members, err := c.ListAudienceMembers(ctx, audienceID)
	if err != nil {
		t.Fatalf("ListAudienceMembers: %v", err)
	}
	if data, ok := members["data"].([]any); !ok || len(data) != 1 {
		t.Fatalf("ListAudienceMembers: expected 1 member, got %+v", members)
	}

	preview2, err := c.PreviewBroadcast(ctx, JSON{"audience_id": audienceID, "template_id": templateID})
	if err != nil {
		t.Fatalf("PreviewBroadcast: %v", err)
	}
	if preview2["recipients"] == nil {
		t.Fatalf("PreviewBroadcast: expected a recipients count, got %+v", preview2)
	}

	if err := c.RemoveAudienceMember(ctx, audienceID, contactID); err != nil {
		t.Fatalf("RemoveAudienceMember: %v", err)
	}

	supp, err := c.CreateSuppression(ctx, JSON{"email": "sdk-contract-suppress-" + run + "@example.com"})
	if err != nil {
		t.Fatalf("CreateSuppression: %v", err)
	}
	suppID, _ := supp["id"].(string)
	if _, err := c.GetSuppression(ctx, suppID); err != nil {
		t.Fatalf("GetSuppression: %v", err)
	}

	wh, err := c.CreateWebhook(ctx, JSON{"url": "https://example.com/hook", "events": []string{"email.delivered"}})
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	webhookID, _ := wh["id"].(string)
	if _, err := c.ListWebhookDeliveries(ctx, webhookID); err != nil {
		t.Fatalf("ListWebhookDeliveries: %v", err)
	}
	if _, err := c.RotateWebhookSecret(ctx, webhookID); err != nil {
		t.Fatalf("RotateWebhookSecret: %v", err)
	}

	email, err := c.SendEmail(ctx, SendEmailRequest{
		From: "hello@sdk-presend.example.com", To: []string{"dest@example.invalid"},
		Subject: "contract test", Text: "hi",
	})
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if _, err := c.GetEmailEvents(ctx, email.ID); err != nil {
		t.Fatalf("GetEmailEvents: %v", err)
	}
}

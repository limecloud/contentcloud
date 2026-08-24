package localworkspace

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func TestValidateCommerceContentForSubmissionRejectsIncompleteFacts(t *testing.T) {
	base := commerceContentFixture()
	tests := []struct {
		name string
		edit func(*CommerceContentItem)
		code string
	}{
		{name: "duplicate variant ids", edit: func(value *CommerceContentItem) { value.Variants = append(value.Variants, value.Variants[0]) }, code: "COMMERCE_CONTENT_VARIANT_INVALID"},
		{name: "missing product facts", edit: func(value *CommerceContentItem) { value.ProductFacts = nil }, code: "COMMERCE_CONTENT_FIELDS_REQUIRED"},
		{name: "blocked reasons", edit: func(value *CommerceContentItem) { value.BlockedReasons = []string{"claim pending"} }, code: "COMMERCE_CONTENT_BLOCKED"},
		{name: "missing input collection", edit: func(value *CommerceContentItem) { value.MissingInputs = nil }, code: "COMMERCE_CONTENT_COLLECTIONS_REQUIRED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.edit(&value)
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ValidateCommerceContentForSubmission(raw, value.ProjectID)
			var domainErr *fault.Error
			if !errors.As(err, &domainErr) || domainErr.Code != test.code {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}
}

func TestRenderCommerceContentDeliveryIsDeterministic(t *testing.T) {
	value := commerceContentFixture()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	first, err := renderCommerceContentDelivery(raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderCommerceContentDelivery(raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.ItemID != value.ID || first.SchemaID != CommerceContentSchema || first.ContentHash != second.ContentHash || len(first.Files) != 3 || len(second.Files) != 3 {
		t.Fatalf("unexpected deterministic delivery metadata: first=%#v second=%#v", first, second)
	}
	for index := range first.Files {
		if first.Files[index].Format != second.Files[index].Format || first.Files[index].SHA256 != second.Files[index].SHA256 || string(first.Files[index].Body) != string(second.Files[index].Body) {
			t.Fatalf("rendered file %d changed between runs", index)
		}
	}
}

func commerceContentFixture() CommerceContentItem {
	return CommerceContentItem{
		ID: "commerce-item:test", Type: "commerce_content_item", Status: "candidate", SchemaVersion: CommerceContentSchema, Deliverability: "review_ready",
		ProjectID: "project:test", ContentID: "commerce-content:test", ContentBatchID: "commerce-batch:test", Title: "可追溯商品内容", Channel: "douyin", TargetAudience: "需要清晰购买理由的人群",
		ProductFacts: map[string]string{"origin": "China", "net_weight": "500g"}, OfferPoints: []string{"verified fact", "clear offer"},
		Variants: []CommerceVariant{{ID: "variant:1", Format: "short_video_caption", Copy: "基于已验证事实的内容变体", CTA: "查看详情"}}, BlockedReasons: []string{}, MissingInputs: []string{},
	}
}

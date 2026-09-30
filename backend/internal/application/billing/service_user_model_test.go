package billing

import (
	"errors"
	"testing"
)

func TestPrivateModelUsageNeedsNoPlatformPriceOrBalance(t *testing.T) {
	service := NewService(&billingRepositoryStub{mode: "usage"})
	service.SetPlatformModelIdentityResolver(modelIdentityResolverStub{identity: PlatformModelIdentity{PlatformModelID: 10, OwnerUserID: 7}})
	authorization, err := service.AuthorizeUsage(t.Context(), 7, "private", "request-1")
	if err != nil || authorization.Mode != "self" {
		t.Fatalf("private authorization: %#v %v", authorization, err)
	}
	ledger, err := service.BuildUsageLedger(t.Context(), UsagePricingInput{
		UserID: 7, PlatformModelName: "private", InputTokens: 123, OutputTokens: 45, Authorization: authorization,
	})
	if err != nil || ledger.BilledNanousd != 0 || ledger.InputTokens != 123 || ledger.OutputTokens != 45 {
		t.Fatalf("private usage ledger: %#v %v", ledger, err)
	}
	if _, err := service.AuthorizeUsage(t.Context(), 8, "private", "request-2"); !errors.Is(err, ErrModelAccessDenied) {
		t.Fatalf("foreign authorization: %v", err)
	}
	if _, err := service.BuildUsageLedger(t.Context(), UsagePricingInput{UserID: 8, PlatformModelName: "private"}); !errors.Is(err, ErrModelAccessDenied) {
		t.Fatalf("foreign ledger: %v", err)
	}
}

package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// Deserializes the exact payloads GET /account/stripe-billing-status returns.
//
// Offline by design — no environment, no credentials. What it checks is a property of the
// generated code, so running it against a live server would only add flakiness. It belongs with
// the generated client rather than the schema tests because the failure it guards against is
// invisible from TypeScript: a schema that validates fine can still generate a client that cannot
// tell one variant from another. StripeBillingStatusResponse used to be a single flat object with
// portal_url and reason both optional, and callers guessed which was populated by null-checking.
//
// How the union picks a branch is a property of scripts/patch-unions, so those assertions live in
// testdata/stripe_union_test.go and run against a freshly patched copy of the generated package.

const (
	enabledJSON  = `{"status":"enabled","billing_method":"stripe","portal_url":"https://billing.stripe.com/p/session_1"}`
	disabledJSON = `{"status":"disabled","billing_method":null,"reason":"No billing method configured"}`
)

func TestDeserializesTheEnabledBranchIntoItsOwnType(t *testing.T) {
	var response openapi.StripeBillingStatusResponse
	if err := json.Unmarshal([]byte(enabledJSON), &response); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if response.StripeBillingEnabled == nil {
		t.Fatalf("expected the enabled branch, got %#v", response.GetActualInstance())
	}
	if response.StripeBillingDisabled != nil {
		t.Fatalf("both branches matched; the discriminator did not select one")
	}
	// Compare the wire value: nullable-string getters differ across generated contracts.
	got, err := json.Marshal(response.StripeBillingEnabled.GetPortalUrl())
	if err != nil {
		t.Fatalf("marshal portal_url: %v", err)
	}
	if string(got) != `"https://billing.stripe.com/p/session_1"` {
		t.Fatalf("portal_url = %s", got)
	}
	if got := response.StripeBillingEnabled.Status; got != "enabled" {
		t.Fatalf("status = %q", got)
	}
}

func TestDeserializesTheDisabledBranchWithANullBillingMethod(t *testing.T) {
	var response openapi.StripeBillingStatusResponse
	if err := json.Unmarshal([]byte(disabledJSON), &response); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if response.StripeBillingDisabled == nil {
		t.Fatalf("expected the disabled branch, got %#v", response.GetActualInstance())
	}
	if got := response.StripeBillingDisabled.Reason; got != "No billing method configured" {
		t.Fatalf("reason = %q", got)
	}
	// Nullable rather than omitted: the disabled branch always sends the key, so a client can tell
	// "no billing method" apart from "field absent".
	if response.StripeBillingDisabled.BillingMethod.Get() != nil {
		t.Fatalf("billing_method should be null, got %v", response.StripeBillingDisabled.BillingMethod.Get())
	}
}

func TestRoundTripsWithoutLosingTheBranch(t *testing.T) {
	var response openapi.StripeBillingStatusResponse
	if err := json.Unmarshal([]byte(enabledJSON), &response); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	serialized, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var again openapi.StripeBillingStatusResponse
	if err := json.Unmarshal(serialized, &again); err != nil {
		t.Fatalf("re-unmarshal failed: %v (payload %s)", err, serialized)
	}
	if again.StripeBillingEnabled == nil {
		t.Fatalf("branch lost on round trip: %s", serialized)
	}
}

func TestEnumsAreSharedNamedTypes(t *testing.T) {
	// The schemas give BillingMethod/AccountType/QuotaTier an `id` so they hoist into named
	// components. Inline enums would generate an anonymous type per property instead, and every
	// account field carrying a billing method would have an incompatible one.
	if got := string(openapi.BILLINGMETHOD_STRIPE); got != "stripe" {
		t.Fatalf("BILLINGMETHOD_STRIPE = %q", got)
	}
}

func TestAccountKeyBillingStatusHasNoPortal(t *testing.T) {
	var response openapi.StripeBillingStatusResponse
	if err := json.Unmarshal([]byte(`{"status":"enabled","billing_method":"stripe","portal_url":null}`), &response); err != nil {
		t.Fatal(err)
	}
	if response.StripeBillingEnabled == nil || response.StripeBillingEnabled.PortalUrl.Get() != nil {
		t.Fatalf("expected enabled billing without a portal: %#v", response.GetActualInstance())
	}
}

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
// Run via ../scripts/generate-and-test.sh, which generates the client the way
// vertesia/vertesia-client-go does — including its patch-openapi-permissive-decode.sh. That
// matters here: the patch strips DisallowUnknownFields() out of the generated decoders, so an
// extra field no longer disqualifies a branch. What separates the branches in the shipped client
// is required-property presence (portal_url for enabled, reason for disabled), which is why
// TestRejectsAMixOfBothBranches supplies both.

const (
	enabledJSON  = `{"status":"enabled","billing_method":"stripe","portal_url":"https://billing.stripe.com/p/session_1"}`
	disabledJSON = `{"status":"disabled","billing_method":null,"reason":"No billing method configured"}`
	// Satisfies the required properties of both branches. The flat model accepted it; a real
	// oneOf must not, and the server never sends portal_url and reason together.
	mixedJSON = `{"status":"enabled","billing_method":"stripe","portal_url":"https://x","reason":"nope"}`
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

func TestRejectsAMixOfBothBranches(t *testing.T) {
	var response openapi.StripeBillingStatusResponse
	if err := json.Unmarshal([]byte(mixedJSON), &response); err == nil {
		t.Fatalf("expected an error for a payload matching both branches, got %#v", response.GetActualInstance())
	}
}

// KNOWN LIMITATION, pinned deliberately: the generated Go client does not enforce the
// discriminator.
//
// The tests above prove real server payloads deserialize correctly. This one documents that they
// do so for a weaker reason than it looks — the client picks a branch by which required properties
// are present and never reads `status`. Two things combine to cause it: the Go template's oneOf
// UnmarshalJSON has no discriminator dispatch, and the generator ignores the `const` on `status`,
// emitting a plain unchecked `string` field. (The Java client does reject these, because its
// generator turns the same `const` into a StatusEnum; see the Java test.)
//
// Two spec-side fixes were tried and rejected, so nobody spends the afternoon again:
//
//   - `enum: ["enabled"]` inline instead of `const`. No effect — the Go generator only builds
//     validating enum types for *named component* schemas, not inline ones.
//   - Hoisting the discriminator into named one-value components (StripeBillingEnabledStatus etc.).
//     This does generate a typed enum, but the client repo's generator config sets
//     enumUnknownDefaultCase: true, so the generated UnmarshalJSON ends with
//     `*v = ..._UNKNOWN_DEFAULT_OPEN_API; return nil` and can never fail. That option is a
//     deliberate forward-compatibility policy — a new server-side enum value must not break already
//     released clients — and it is not ours to override from the spec. It also makes the outcome
//     strictly worse: "disabled" is silently rewritten to "unknown_default_open_api", destroying the
//     wire value that the plain `string` field below preserves. Two extra public components for a
//     regression is a bad trade.
//
// The consequence is bounded, though not eliminated. Every branch of the handler is checked against
// the canonical schema before emission — checkResponseContract(ctx, 'StripeBillingStatusResponse'),
// and AJV does honour `const`, so a contradictory body matches neither branch of the oneOf. That
// makes server-side drift DETECTED (logged, and a hard failure in local development); outside local
// development the response is still sent. So the protection is a fast diagnosis, not a barrier, and
// this test covers what remains: a client that receives a contradictory payload anyway — from a
// regressed server, or from an environment where the log went unread — silently misreads it as the
// enabled branch instead of failing.
//
// Asserted as accept-with-wrong-branch rather than skipped, so that whenever the generator or the
// client repo's patch changes this behaviour, this test fails and someone tightens it into the
// rejection assertion it should eventually be.
func TestDoesNotEnforceTheDiscriminator(t *testing.T) {
	// Says disabled, carries the enabled branch's required property: decoded as ENABLED, keeping
	// the contradictory status string verbatim.
	var mismatched openapi.StripeBillingStatusResponse
	body := `{"status":"disabled","billing_method":"stripe","portal_url":"https://x"}`
	if err := json.Unmarshal([]byte(body), &mismatched); err != nil {
		t.Fatalf("behaviour changed — the client now rejects a contradictory discriminator (%v). "+
			"Tighten this test into a rejection assertion.", err)
	}
	if mismatched.StripeBillingEnabled == nil {
		t.Fatalf("expected the enabled branch to be chosen structurally, got %#v", mismatched.GetActualInstance())
	}
	if got := mismatched.StripeBillingEnabled.Status; got != "disabled" {
		t.Fatalf("expected the contradictory status to survive unchecked, got %q", got)
	}

	// Neither branch's required property is present, so neither matches — this the client does
	// catch, because required-property presence is the mechanism it actually uses.
	var unmatched openapi.StripeBillingStatusResponse
	if err := json.Unmarshal([]byte(`{"status":"enabled","billing_method":"stripe"}`), &unmatched); err == nil {
		t.Fatalf("expected an error when neither branch matches, got %#v", unmatched.GetActualInstance())
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

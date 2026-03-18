package proxy

import (
	"strings"
	"testing"
	"time"
)

func TestDescribeRequestOutcome_IncompleteResponse(t *testing.T) {
	view := DescribeRequestOutcome(RequestOutcomeEvent{
		Provider: "qaq",
		Status:   200,
		Delivery: string(deliveryCommittedComplete),
		Protocol: string(protocolIncomplete),
	})

	if view.Result != "incomplete_response" {
		t.Fatalf("result: got %q want %q", view.Result, "incomplete_response")
	}
	if view.Label != "Incomplete response via qaq" {
		t.Fatalf("label: got %q want %q", view.Label, "Incomplete response via qaq")
	}
	if !strings.Contains(view.Detail, "ended the response before completion") {
		t.Fatalf("detail: got %q", view.Detail)
	}
}

func TestDescribeProviderAvailability_States(t *testing.T) {
	cooling := DescribeProviderAvailability("qaq", true, ProviderRuntimeSnapshot{
		Name:              "qaq",
		DeactivatedReason: "rate_limit",
		DeactivatedUntil:  time.Now().Add(30 * time.Second),
	})
	if cooling.State != "cooling_down" {
		t.Fatalf("cooling.State: got %q want %q", cooling.State, "cooling_down")
	}
	if cooling.Label != "qaq (cooling down)" {
		t.Fatalf("cooling.Label: got %q want %q", cooling.Label, "qaq (cooling down)")
	}

	probe := DescribeProviderAvailability("qaq", true, ProviderRuntimeSnapshot{
		Name:         "qaq",
		CircuitState: "half_open",
	})
	if probe.State != "recovery_probe" {
		t.Fatalf("probe.State: got %q want %q", probe.State, "recovery_probe")
	}
	if probe.Label != "qaq (recovery probe)" {
		t.Fatalf("probe.Label: got %q want %q", probe.Label, "qaq (recovery probe)")
	}

	noKeys := DescribeProviderAvailability("qaq", true, ProviderRuntimeSnapshot{
		Name:              "qaq",
		KeyCount:          2,
		AvailableKeyCount: 0,
	})
	if noKeys.State != "unavailable" {
		t.Fatalf("noKeys.State: got %q want %q", noKeys.State, "unavailable")
	}
	if noKeys.Label != "qaq (no keys available)" {
		t.Fatalf("noKeys.Label: got %q want %q", noKeys.Label, "qaq (no keys available)")
	}
}

func TestDescribeRequestOutcome_ExplicitFailure(t *testing.T) {
	view := DescribeRequestOutcome(RequestOutcomeEvent{
		Result:   "all_providers_failed",
		Provider: "qaq",
		Detail:   "qaq returned HTTP 503 Service Unavailable",
	})

	if view.Result != "all_providers_failed" {
		t.Fatalf("result: got %q want %q", view.Result, "all_providers_failed")
	}
	if view.Label != "All providers failed" {
		t.Fatalf("label: got %q want %q", view.Label, "All providers failed")
	}
	if !strings.Contains(view.Detail, "503") {
		t.Fatalf("detail: got %q", view.Detail)
	}
}

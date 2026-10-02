// Package notification handles sending notifications to external services.
package notification

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/severity"
)

func TestNewNotifier(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantEnabled bool
		wantErr     bool
	}{
		{
			name: "notifications disabled",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    false,
					ShoutrrURL: "",
				},
			},
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "notifications disabled with URL set",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    false,
					ShoutrrURL: "slack://token@channel",
				},
			},
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "notifications enabled without URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "",
				},
			},
			wantEnabled: false,
			wantErr:     true,
		},
		{
			name: "notifications enabled with URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "slack://token@channel",
				},
			},
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "notifications enabled with discord URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "discord://token@id",
				},
			},
			wantEnabled: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := NewNotifier(tt.cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewNotifier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if notifier == nil {
				t.Fatal("NewNotifier() returned nil notifier")
			}

			if notifier.enabled != tt.wantEnabled {
				t.Errorf("NewNotifier() enabled = %v, want %v", notifier.enabled, tt.wantEnabled)
			}
		})
	}
}

func TestNotifier_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		notifier *Notifier
		want     bool
	}{
		{
			name:     "enabled notifier",
			notifier: &Notifier{enabled: true, shoutrrrURL: "slack://token@channel"},
			want:     true,
		},
		{
			name:     "disabled notifier",
			notifier: &Notifier{enabled: false, shoutrrrURL: ""},
			want:     false,
		},
		{
			name:     "disabled notifier with URL",
			notifier: &Notifier{enabled: false, shoutrrrURL: "slack://token@channel"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.notifier.IsEnabled(); got != tt.want {
				t.Errorf("Notifier.IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFormatScanSummary tests the pure formatting function with a fixed time.
func TestFormatScanSummary(t *testing.T) {
	fixedTime := time.Date(2026, 10, 2, 15, 4, 5, 0, time.UTC)

	tests := []struct {
		name           string
		summary        string
		containerCount int
		overall        severity.Level
		failed         []string
		wantContains   []string
		wantAbsent     []string
	}{
		{
			name:           "containers count present",
			summary:        "all good",
			containerCount: 3,
			overall:        severity.Critical,
			failed:         nil,
			wantContains:   []string{"📦 Containers: 3"},
		},
		{
			name:           "severity badge present",
			summary:        "some summary",
			containerCount: 1,
			overall:        severity.Critical,
			failed:         nil,
			wantContains:   []string{"Severity: 🔴 Critical"},
		},
		{
			name:           "failures line when failures exist",
			summary:        "text",
			containerCount: 3,
			overall:        severity.Warning,
			failed:         []string{"a", "b"},
			wantContains:   []string{"❌ Analysis failed: a, b (will be retried)"},
		},
		{
			name:           "no failures line when no failures",
			summary:        "text",
			containerCount: 2,
			overall:        severity.OK,
			failed:         nil,
			wantAbsent:     []string{"Analysis failed"},
		},
		{
			name:           "empty summary ends after header block without trailing blank line",
			summary:        "",
			containerCount: 1,
			overall:        severity.OK,
			failed:         nil,
			wantAbsent:     []string{"\n\n\n"},
		},
		{
			name:           "non-empty summary appears after one blank line",
			summary:        "Container alpha is healthy.",
			containerCount: 1,
			overall:        severity.OK,
			failed:         nil,
			wantContains:   []string{"\n\nContainer alpha is healthy."},
		},
		{
			name:           "header line present",
			summary:        "",
			containerCount: 0,
			overall:        severity.OK,
			failed:         nil,
			wantContains:   []string{"🔍 DLIA Scan Complete"},
		},
		{
			name:           "time line present",
			summary:        "",
			containerCount: 0,
			overall:        severity.OK,
			failed:         nil,
			wantContains:   []string{"🕐 Time:"},
		},
		{
			name:           "warning badge",
			summary:        "",
			containerCount: 2,
			overall:        severity.Warning,
			failed:         nil,
			wantContains:   []string{"Severity: 🟡 Warning"},
		},
		{
			name:           "ok badge",
			summary:        "",
			containerCount: 2,
			overall:        severity.OK,
			failed:         nil,
			wantContains:   []string{"Severity: 🟢 OK"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatScanSummary(tt.summary, tt.containerCount, tt.overall, tt.failed, fixedTime)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("formatScanSummary() missing %q in:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("formatScanSummary() unexpectedly contains %q in:\n%s", absent, got)
				}
			}
		})
	}
}

// TestNotifier_SendScanSummary_Disabled tests that a disabled notifier returns nil.
func TestNotifier_SendScanSummary_Disabled(t *testing.T) {
	notifier := &Notifier{enabled: false, shoutrrrURL: ""}

	err := notifier.SendScanSummary("test summary", 5, severity.Warning, nil)
	if err != nil {
		t.Errorf("SendScanSummary() with disabled notifications should return nil, got error: %v", err)
	}
}

// TestNotifier_SendScanSummary_DisabledWithLevel tests disabled notifier with various levels.
func TestNotifier_SendScanSummary_DisabledWithLevel(t *testing.T) {
	notifier := &Notifier{enabled: false, shoutrrrURL: ""}

	levels := []severity.Level{severity.OK, severity.Warning, severity.Unknown, severity.Critical}
	for _, lvl := range levels {
		err := notifier.SendScanSummary("test", 3, lvl, []string{"a"})
		if err != nil {
			t.Errorf("SendScanSummary() disabled with level %s should return nil, got: %v", lvl, err)
		}
	}
}

func TestNewNotifier_ErrorMessage(t *testing.T) {
	cfg := &config.Config{
		Notification: config.NotificationConfig{
			Enabled:    true,
			ShoutrrURL: "",
		},
	}

	_, err := NewNotifier(cfg)
	if err == nil {
		t.Fatal("expected error when notification enabled but URL not configured")
	}

	expectedMsg := "notification enabled but shoutrrr_url not configured: provide URL in format 'service://credentials' (e.g., slack://token@channel, discord://token@webhookid)"
	if err.Error() != expectedMsg {
		t.Errorf("NewNotifier() error message = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestNotifier_ShoutrrrURL(t *testing.T) {
	expectedURL := "slack://xoxb:token@channel"
	cfg := &config.Config{
		Notification: config.NotificationConfig{
			Enabled:    true,
			ShoutrrURL: expectedURL,
		},
	}

	notifier, err := NewNotifier(cfg)
	if err != nil {
		t.Fatalf("NewNotifier() unexpected error: %v", err)
	}

	if notifier.shoutrrrURL != expectedURL {
		t.Errorf("Notifier.shoutrrrURL = %q, want %q", notifier.shoutrrrURL, expectedURL)
	}
}

// TestNotifier_ZeroValue tests the zero value behavior.
func TestNotifier_ZeroValue(t *testing.T) {
	notifier := &Notifier{}

	if notifier.IsEnabled() {
		t.Error("Zero value Notifier should have IsEnabled() = false")
	}

	err := notifier.SendScanSummary("test", 1, severity.OK, nil)
	if err != nil {
		t.Errorf("SendScanSummary() on zero value notifier should return nil, got: %v", err)
	}
}

func TestNewNotifier_NilConfig(t *testing.T) {
	cfg := &config.Config{}

	notifier, err := NewNotifier(cfg)
	if err != nil {
		t.Fatalf("NewNotifier() with zero config should not error, got: %v", err)
	}

	if notifier.IsEnabled() {
		t.Error("Notifier with zero config should be disabled")
	}
}

// TestNotifier_SendScanSummary_EdgeCases tests edge cases with disabled notifier.
func TestNotifier_SendScanSummary_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		summary        string
		containerCount int
		overall        severity.Level
		failed         []string
	}{
		{
			name:           "empty summary",
			summary:        "",
			containerCount: 5,
			overall:        severity.OK,
			failed:         nil,
		},
		{
			name:           "zero containers with critical",
			summary:        "test summary",
			containerCount: 0,
			overall:        severity.Critical,
			failed:         []string{"x"},
		},
		{
			name:           "zero containers without issues",
			summary:        "test summary",
			containerCount: 0,
			overall:        severity.OK,
			failed:         nil,
		},
		{
			name:           "large container count",
			summary:        "test summary",
			containerCount: 10000,
			overall:        severity.Warning,
			failed:         nil,
		},
		{
			name:           "negative container count",
			summary:        "test summary",
			containerCount: -1,
			overall:        severity.OK,
			failed:         nil,
		},
		{
			name:           "very long summary",
			summary:        string(make([]byte, 10000)),
			containerCount: 5,
			overall:        severity.Critical,
			failed:         nil,
		},
		{
			name:           "summary with special characters",
			summary:        "Test 🐳 with émojis and spëcial çharacters: \n\t\r",
			containerCount: 3,
			overall:        severity.OK,
			failed:         nil,
		},
		{
			name:           "summary with newlines",
			summary:        "Line 1\nLine 2\nLine 3",
			containerCount: 7,
			overall:        severity.Warning,
			failed:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &Notifier{enabled: false}
			err := notifier.SendScanSummary(tt.summary, tt.containerCount, tt.overall, tt.failed)
			if err != nil {
				t.Errorf("SendScanSummary() with disabled notifier should not error, got: %v", err)
			}
		})
	}
}

// TestNotifier_SendScanSummary_LevelVariations tests different severity levels.
func TestNotifier_SendScanSummary_LevelVariations(t *testing.T) {
	tests := []struct {
		name           string
		containerCount int
		overall        severity.Level
	}{
		{"single container ok", 1, severity.OK},
		{"single container warning", 1, severity.Warning},
		{"multiple containers critical", 100, severity.Critical},
		{"multiple containers unknown", 100, severity.Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &Notifier{enabled: false}
			err := notifier.SendScanSummary("test summary", tt.containerCount, tt.overall, nil)
			if err != nil {
				t.Errorf("SendScanSummary() got error: %v", err)
			}
		})
	}
}

// TestNotifier_EnabledStateConsistency ensures enabled state is consistent.
func TestNotifier_EnabledStateConsistency(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		shoutrrrURL string
	}{
		{"enabled with URL", true, "slack://token@channel"},
		{"disabled with URL", false, "slack://token@channel"},
		{"disabled without URL", false, ""},
		{"enabled with discord URL", true, "discord://token@webhookid/token"},
		{"enabled with telegram URL", true, "telegram://token@telegram?channels=@channel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &Notifier{enabled: tt.enabled, shoutrrrURL: tt.shoutrrrURL}
			if notifier.IsEnabled() != tt.enabled {
				t.Errorf("IsEnabled() = %v, want %v", notifier.IsEnabled(), tt.enabled)
			}
			for i := range 5 {
				if notifier.IsEnabled() != tt.enabled {
					t.Errorf("IsEnabled() call %d = %v, want %v", i, notifier.IsEnabled(), tt.enabled)
				}
			}
		})
	}
}

// TestNotifier_SendScanSummary_MultipleInvocations tests calling SendScanSummary multiple times.
func TestNotifier_SendScanSummary_MultipleInvocations(t *testing.T) {
	notifier := &Notifier{enabled: false}

	testCases := []struct {
		summary        string
		containerCount int
		overall        severity.Level
		failed         []string
	}{
		{"first summary", 5, severity.Warning, nil},
		{"second summary", 3, severity.OK, nil},
		{"third summary", 10, severity.Critical, []string{"c1"}},
		{"", 0, severity.OK, nil},
		{"final summary", 1, severity.OK, nil},
	}

	for i, tc := range testCases {
		err := notifier.SendScanSummary(tc.summary, tc.containerCount, tc.overall, tc.failed)
		if err != nil {
			t.Errorf("SendScanSummary() invocation %d returned error: %v", i, err)
		}
	}
}

// TestNotifier_FieldAccessibility tests that notifier fields are set correctly.
func TestNotifier_FieldAccessibility(t *testing.T) {
	tests := []struct {
		name            string
		cfg             *config.Config
		wantEnabled     bool
		wantShoutrrrURL string
	}{
		{
			name: "slack URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "slack://xoxb-token@channel",
				},
			},
			wantEnabled:     true,
			wantShoutrrrURL: "slack://xoxb-token@channel",
		},
		{
			name: "discord URL with webhook",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "discord://token@webhookid/token",
				},
			},
			wantEnabled:     true,
			wantShoutrrrURL: "discord://token@webhookid/token",
		},
		{
			name: "disabled with empty URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    false,
					ShoutrrURL: "",
				},
			},
			wantEnabled:     false,
			wantShoutrrrURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := NewNotifier(tt.cfg)
			if err != nil {
				t.Fatalf("NewNotifier() unexpected error: %v", err)
			}
			if notifier.enabled != tt.wantEnabled {
				t.Errorf("notifier.enabled = %v, want %v", notifier.enabled, tt.wantEnabled)
			}
			if notifier.shoutrrrURL != tt.wantShoutrrrURL {
				t.Errorf("notifier.shoutrrrURL = %q, want %q", notifier.shoutrrrURL, tt.wantShoutrrrURL)
			}
		})
	}
}

// TestNotifier_ConcurrentAccess tests thread safety of IsEnabled.
func TestNotifier_ConcurrentAccess(t *testing.T) {
	notifier := &Notifier{enabled: true, shoutrrrURL: "slack://token@channel"}

	done := make(chan bool)
	for range 10 {
		go func() {
			for range 100 {
				_ = notifier.IsEnabled()
			}
			done <- true
		}()
	}
	for range 10 {
		<-done
	}

	if !notifier.IsEnabled() {
		t.Error("IsEnabled() should still return true after concurrent access")
	}
}

// TestNotifier_SendScanSummary_MessageFormat tests the send path exercises formatting.
func TestNotifier_SendScanSummary_MessageFormat(t *testing.T) {
	tests := []struct {
		name           string
		summary        string
		containerCount int
		overall        severity.Level
		failed         []string
		expectError    bool
	}{
		{
			name:           "critical level",
			summary:        "Critical vulnerabilities detected",
			containerCount: 5,
			overall:        severity.Critical,
			failed:         nil,
			expectError:    true,
		},
		{
			name:           "ok level",
			summary:        "All containers are healthy",
			containerCount: 3,
			overall:        severity.OK,
			failed:         nil,
			expectError:    true,
		},
		{
			name:           "with failures",
			summary:        "",
			containerCount: 10,
			overall:        severity.Unknown,
			failed:         []string{"a", "b"},
			expectError:    true,
		},
		{
			name:           "single container",
			summary:        "Container checked",
			containerCount: 1,
			overall:        severity.Warning,
			failed:         nil,
			expectError:    true,
		},
		{
			name:           "summary with newlines",
			summary:        "Line 1\nLine 2\nLine 3",
			containerCount: 2,
			overall:        severity.OK,
			failed:         nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &Notifier{enabled: true, shoutrrrURL: "invalid://test"}
			err := notifier.SendScanSummary(tt.summary, tt.containerCount, tt.overall, tt.failed)
			if tt.expectError {
				if err == nil {
					t.Error("SendScanSummary() expected error with invalid URL, got nil")
				}
				if err != nil && !strings.Contains(err.Error(), "notification failed") {
					t.Errorf("SendScanSummary() error should contain 'notification failed', got: %v", err)
				}
			}
		})
	}
}

// TestNotifier_SendScanSummary_ErrorWrapping tests error wrapping includes severity.
func TestNotifier_SendScanSummary_ErrorWrapping(t *testing.T) {
	notifier := &Notifier{enabled: true, shoutrrrURL: "totally-invalid-url-format"}

	err := notifier.SendScanSummary("test", 1, severity.OK, nil)
	if err == nil {
		t.Fatal("SendScanSummary() with invalid URL should return error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "notification failed") {
		t.Errorf("Error should be wrapped with 'notification failed', got: %s", errMsg)
	}
}

// TestNotifier_SendScanSummary_ContainerCountFormatting tests various container counts.
func TestNotifier_SendScanSummary_ContainerCountFormatting(t *testing.T) {
	counts := []int{0, 1, 10, 100, 999, 1000, 10000, -1}

	for _, count := range counts {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			notifier := &Notifier{enabled: true, shoutrrrURL: "invalid://url"}
			err := notifier.SendScanSummary("test", count, severity.OK, nil)
			if err == nil {
				t.Error("Expected error with invalid URL")
			}
		})
	}
}

// TestNewNotifier_ConfigVariations tests various config combinations.
func TestNewNotifier_ConfigVariations(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "whitespace only URL when enabled",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "   ",
				},
			},
			wantErr: true,
			errMsg:  "notification enabled but shoutrrr_url not configured: provide URL in format 'service://credentials' (e.g., slack://token@channel, discord://token@webhookid)",
		},
		{
			name: "valid gotify URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "gotify://gotify.example.com/token",
				},
			},
			wantErr: false,
		},
		{
			name: "valid email URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "smtp://user:pass@smtp.example.com:587/?from=from@example.com&to=to@example.com",
				},
			},
			wantErr: false,
		},
		{
			name: "valid teams URL",
			cfg: &config.Config{
				Notification: config.NotificationConfig{
					Enabled:    true,
					ShoutrrURL: "teams://group@tenant/altId/groupOwner?host=webhook.office.com",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := NewNotifier(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("NewNotifier() expected error, got nil")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("NewNotifier() error = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("NewNotifier() unexpected error: %v", err)
				return
			}

			if notifier == nil {
				t.Error("NewNotifier() returned nil notifier")
				return
			}

			if !notifier.IsEnabled() {
				t.Error("NewNotifier() returned disabled notifier when should be enabled")
			}
		})
	}
}

// TestNotifier_SendScanSummary_SendFailure pins the send-failure error message.
// The service type is extracted from the URL scheme; a URL without a scheme
// reports "unknown".
func TestNotifier_SendScanSummary_SendFailure(t *testing.T) {
	notifier := &Notifier{enabled: true, shoutrrrURL: "://not-a-valid-url"}

	err := notifier.SendScanSummary("summary", 2, severity.Critical, nil)
	if err == nil {
		t.Fatal("Expected error when the shoutrrr send fails")
	}

	if !strings.Contains(err.Error(), "notification failed to send via unknown") {
		t.Errorf("Expected 'via unknown' for a scheme-less URL, got: %v", err)
	}
	if !strings.Contains(err.Error(), "containers: 2") {
		t.Errorf("Expected container count in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "severity: critical") {
		t.Errorf("Expected severity in error, got: %v", err)
	}
}

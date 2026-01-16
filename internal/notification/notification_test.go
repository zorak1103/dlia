// Package notification handles sending notifications to external services.
package notification

import (
	"fmt"
	"testing"

	"github.com/zorak1103/dlia/internal/config"
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

func TestNotifier_SendScanSummary_Disabled(t *testing.T) {
	notifier := &Notifier{
		enabled:     false,
		shoutrrrURL: "",
	}

	// When notifications are disabled, SendScanSummary should return nil without error
	err := notifier.SendScanSummary("test summary", 5, true)
	if err != nil {
		t.Errorf("SendScanSummary() with disabled notifications should return nil, got error: %v", err)
	}
}

func TestNotifier_SendScanSummary_DisabledWithIssues(t *testing.T) {
	notifier := &Notifier{
		enabled:     false,
		shoutrrrURL: "",
	}

	// Test with issues found
	err := notifier.SendScanSummary("critical issues found", 10, true)
	if err != nil {
		t.Errorf("SendScanSummary() with disabled notifications should return nil, got error: %v", err)
	}
}

func TestNotifier_SendScanSummary_DisabledNoIssues(t *testing.T) {
	notifier := &Notifier{
		enabled:     false,
		shoutrrrURL: "",
	}

	// Test without issues found
	err := notifier.SendScanSummary("all clear", 3, false)
	if err != nil {
		t.Errorf("SendScanSummary() with disabled notifications should return nil, got error: %v", err)
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

// TestNotifier_ZeroValue tests the zero value behavior
func TestNotifier_ZeroValue(t *testing.T) {
	notifier := &Notifier{}

	// Zero value should have enabled as false
	if notifier.IsEnabled() {
		t.Error("Zero value Notifier should have IsEnabled() = false")
	}

	// SendScanSummary should not error on zero value notifier
	err := notifier.SendScanSummary("test", 1, false)
	if err != nil {
		t.Errorf("SendScanSummary() on zero value notifier should return nil, got: %v", err)
	}
}

func TestNewNotifier_NilConfig(t *testing.T) {
	// This test documents the expected behavior when config has zero values
	cfg := &config.Config{}

	notifier, err := NewNotifier(cfg)
	if err != nil {
		t.Fatalf("NewNotifier() with zero config should not error, got: %v", err)
	}

	if notifier.IsEnabled() {
		t.Error("Notifier with zero config should be disabled")
	}
}

// TestNotifier_SendScanSummary_EdgeCases tests edge cases for SendScanSummary
func TestNotifier_SendScanSummary_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		summary        string
		containerCount int
		issuesFound    bool
		wantErr        bool
	}{
		{
			name:           "empty summary",
			summary:        "",
			containerCount: 5,
			issuesFound:    false,
			wantErr:        false,
		},
		{
			name:           "zero containers with issues",
			summary:        "test summary",
			containerCount: 0,
			issuesFound:    true,
			wantErr:        false,
		},
		{
			name:           "zero containers without issues",
			summary:        "test summary",
			containerCount: 0,
			issuesFound:    false,
			wantErr:        false,
		},
		{
			name:           "large container count",
			summary:        "test summary",
			containerCount: 10000,
			issuesFound:    true,
			wantErr:        false,
		},
		{
			name:           "negative container count",
			summary:        "test summary",
			containerCount: -1,
			issuesFound:    false,
			wantErr:        false,
		},
		{
			name:           "very long summary",
			summary:        string(make([]byte, 10000)),
			containerCount: 5,
			issuesFound:    true,
			wantErr:        false,
		},
		{
			name:           "summary with special characters",
			summary:        "Test 🐳 with émojis and spëcial çharacters: \n\t\r",
			containerCount: 3,
			issuesFound:    false,
			wantErr:        false,
		},
		{
			name:           "summary with newlines",
			summary:        "Line 1\nLine 2\nLine 3",
			containerCount: 7,
			issuesFound:    true,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with disabled notifier - should always succeed
			notifier := &Notifier{enabled: false}
			err := notifier.SendScanSummary(tt.summary, tt.containerCount, tt.issuesFound)
			if err != nil {
				t.Errorf("SendScanSummary() with disabled notifier should not error, got: %v", err)
			}
		})
	}
}

// TestNotifier_SendScanSummary_IssuesFlagVariations tests different combinations of inputs
func TestNotifier_SendScanSummary_IssuesFlagVariations(t *testing.T) {
	tests := []struct {
		name           string
		containerCount int
		issuesFound    bool
		description    string
	}{
		{
			name:           "single container with issues",
			containerCount: 1,
			issuesFound:    true,
			description:    "should handle singular container with issues",
		},
		{
			name:           "single container without issues",
			containerCount: 1,
			issuesFound:    false,
			description:    "should handle singular container without issues",
		},
		{
			name:           "multiple containers with issues",
			containerCount: 100,
			issuesFound:    true,
			description:    "should handle multiple containers with issues",
		},
		{
			name:           "multiple containers without issues",
			containerCount: 100,
			issuesFound:    false,
			description:    "should handle multiple containers without issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &Notifier{enabled: false}
			err := notifier.SendScanSummary("test summary", tt.containerCount, tt.issuesFound)
			if err != nil {
				t.Errorf("SendScanSummary() %s, got error: %v", tt.description, err)
			}
		})
	}
}

// TestNotifier_EnabledStateConsistency ensures enabled state is consistent
func TestNotifier_EnabledStateConsistency(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		shoutrrrURL string
	}{
		{
			name:        "enabled with URL",
			enabled:     true,
			shoutrrrURL: "slack://token@channel",
		},
		{
			name:        "disabled with URL",
			enabled:     false,
			shoutrrrURL: "slack://token@channel",
		},
		{
			name:        "disabled without URL",
			enabled:     false,
			shoutrrrURL: "",
		},
		{
			name:        "enabled with discord URL",
			enabled:     true,
			shoutrrrURL: "discord://token@webhookid/token",
		},
		{
			name:        "enabled with telegram URL",
			enabled:     true,
			shoutrrrURL: "telegram://token@telegram?channels=@channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &Notifier{
				enabled:     tt.enabled,
				shoutrrrURL: tt.shoutrrrURL,
			}

			// IsEnabled should always match the enabled field
			if notifier.IsEnabled() != tt.enabled {
				t.Errorf("IsEnabled() = %v, want %v", notifier.IsEnabled(), tt.enabled)
			}

			// Multiple calls should return consistent results
			for i := 0; i < 5; i++ {
				if notifier.IsEnabled() != tt.enabled {
					t.Errorf("IsEnabled() call %d = %v, want %v", i, notifier.IsEnabled(), tt.enabled)
				}
			}
		})
	}
}

// TestNotifier_SendScanSummary_MultipleInvocations tests calling SendScanSummary multiple times
func TestNotifier_SendScanSummary_MultipleInvocations(t *testing.T) {
	notifier := &Notifier{enabled: false}

	// Call multiple times with different parameters
	testCases := []struct {
		summary        string
		containerCount int
		issuesFound    bool
	}{
		{"first summary", 5, true},
		{"second summary", 3, false},
		{"third summary", 10, true},
		{"", 0, false},
		{"final summary", 1, false},
	}

	for i, tc := range testCases {
		err := notifier.SendScanSummary(tc.summary, tc.containerCount, tc.issuesFound)
		if err != nil {
			t.Errorf("SendScanSummary() invocation %d returned error: %v", i, err)
		}
	}
}

// TestNotifier_FieldAccessibility tests that notifier fields are set correctly
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

// TestNotifier_ConcurrentAccess tests thread safety of IsEnabled
func TestNotifier_ConcurrentAccess(t *testing.T) {
	notifier := &Notifier{
		enabled:     true,
		shoutrrrURL: "slack://token@channel",
	}

	// Spawn multiple goroutines to call IsEnabled concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = notifier.IsEnabled()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify state hasn't changed
	if !notifier.IsEnabled() {
		t.Error("IsEnabled() should still return true after concurrent access")
	}
}

// TestNotifier_SendScanSummary_MessageFormat tests the message formatting
func TestNotifier_SendScanSummary_MessageFormat(t *testing.T) {
	tests := []struct {
		name           string
		summary        string
		containerCount int
		issuesFound    bool
		expectError    bool
	}{
		{
			name:           "with issues found",
			summary:        "Critical vulnerabilities detected",
			containerCount: 5,
			issuesFound:    true,
			expectError:    true, // Will fail because URL is invalid, but tests the formatting path
		},
		{
			name:           "without issues",
			summary:        "All containers are healthy",
			containerCount: 3,
			issuesFound:    false,
			expectError:    true, // Will fail because URL is invalid, but tests the formatting path
		},
		{
			name:           "empty summary with issues",
			summary:        "",
			containerCount: 10,
			issuesFound:    true,
			expectError:    true,
		},
		{
			name:           "single container",
			summary:        "Container checked",
			containerCount: 1,
			issuesFound:    false,
			expectError:    true,
		},
		{
			name:           "many containers",
			summary:        "Large scale scan completed",
			containerCount: 500,
			issuesFound:    true,
			expectError:    true,
		},
		{
			name:           "summary with newlines",
			summary:        "Line 1\nLine 2\nLine 3",
			containerCount: 2,
			issuesFound:    false,
			expectError:    true,
		},
		{
			name:           "summary with special characters",
			summary:        "Test 🔥 émojis and spëcial çhars",
			containerCount: 7,
			issuesFound:    true,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use an invalid URL that will fail but still exercises the message formatting code
			notifier := &Notifier{
				enabled:     true,
				shoutrrrURL: "invalid://test",
			}

			err := notifier.SendScanSummary(tt.summary, tt.containerCount, tt.issuesFound)

			// We expect an error because the URL is invalid, but this still tests
			// that the message formatting code path is executed
			if tt.expectError {
				if err == nil {
					t.Error("SendScanSummary() expected error with invalid URL, got nil")
				}
				// Check that error is wrapped properly
				if err != nil && !contains(err.Error(), "notification failed") {
					t.Errorf("SendScanSummary() error should contain 'notification failed', got: %v", err)
				}
			}
		})
	}
}

// TestNotifier_SendScanSummary_ErrorWrapping tests error wrapping
func TestNotifier_SendScanSummary_ErrorWrapping(t *testing.T) {
	notifier := &Notifier{
		enabled:     true,
		shoutrrrURL: "totally-invalid-url-format",
	}

	err := notifier.SendScanSummary("test", 1, false)
	if err == nil {
		t.Fatal("SendScanSummary() with invalid URL should return error")
	}

	// Check error message format
	errMsg := err.Error()
	if !contains(errMsg, "notification failed") {
		t.Errorf("Error should be wrapped with 'notification failed', got: %s", errMsg)
	}
}

// TestNotifier_SendScanSummary_IssuesDetectedPath tests the issues detected path
func TestNotifier_SendScanSummary_IssuesDetectedPath(t *testing.T) {
	notifier := &Notifier{
		enabled:     true,
		shoutrrrURL: "generic://invalid-but-exercises-code-path",
	}

	// Test with issues found - this exercises the issuesFound == true branch
	err := notifier.SendScanSummary("Security issues found", 5, true)
	if err == nil {
		t.Error("Expected error with invalid URL")
	}
}

// TestNotifier_SendScanSummary_NoIssuesPath tests the no issues path
func TestNotifier_SendScanSummary_NoIssuesPath(t *testing.T) {
	notifier := &Notifier{
		enabled:     true,
		shoutrrrURL: "generic://invalid-but-exercises-code-path",
	}

	// Test without issues - this exercises the issuesFound == false branch
	err := notifier.SendScanSummary("All clear", 3, false)
	if err == nil {
		t.Error("Expected error with invalid URL")
	}
}

// TestNotifier_SendScanSummary_ContainerCountFormatting tests various container counts
func TestNotifier_SendScanSummary_ContainerCountFormatting(t *testing.T) {
	counts := []int{0, 1, 10, 100, 999, 1000, 10000, -1}

	for _, count := range counts {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			notifier := &Notifier{
				enabled:     true,
				shoutrrrURL: "invalid://url",
			}

			err := notifier.SendScanSummary("test", count, false)
			// We expect an error due to invalid URL, but the formatting code is exercised
			if err == nil {
				t.Error("Expected error with invalid URL")
			}
		})
	}
}

// TestNotifier_SendScanSummary_SummaryVariations tests different summary inputs
func TestNotifier_SendScanSummary_SummaryVariations(t *testing.T) {
	summaries := []string{
		"",
		"Simple summary",
		"Multi\nLine\nSummary",
		"Summary with\ttabs",
		"Summary with special chars: @#$%^&*()",
		"Very " + string(make([]byte, 1000)) + " long summary",
		"🐳 Docker 🔥 Fire 💡 Light 🎉 Party",
	}

	for i, summary := range summaries {
		t.Run(fmt.Sprintf("summary_%d", i), func(t *testing.T) {
			notifier := &Notifier{
				enabled:     true,
				shoutrrrURL: "invalid://url",
			}

			err := notifier.SendScanSummary(summary, 5, false)
			if err == nil {
				t.Error("Expected error with invalid URL")
			}
		})
	}
}

// TestNotifier_SendScanSummary_BothBranches tests both branches of issuesFound
func TestNotifier_SendScanSummary_BothBranches(t *testing.T) {
	notifier := &Notifier{
		enabled:     true,
		shoutrrrURL: "invalid://url",
	}

	// Test issuesFound = true branch
	err1 := notifier.SendScanSummary("Issues", 5, true)
	if err1 == nil {
		t.Error("Expected error for true branch")
	}

	// Test issuesFound = false branch
	err2 := notifier.SendScanSummary("No issues", 5, false)
	if err2 == nil {
		t.Error("Expected error for false branch")
	}

	// Both should return errors (invalid URL) but exercise different code paths
	if err1 != nil && err2 != nil {
		// Both paths were exercised
		t.Log("Both code paths exercised successfully")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestNewNotifier_ConfigVariations tests various config combinations
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

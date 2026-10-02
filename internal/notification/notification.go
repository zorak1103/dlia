// Package notification handles sending notifications to external services.
package notification

import (
	"fmt"
	"strings"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/severity"
)

// Notifier handles sending notifications via Shoutrrr.
type Notifier struct {
	enabled     bool
	shoutrrrURL string
}

// NewNotifier initializes a Shoutrrr-based notification client from config.
func NewNotifier(cfg *config.Config) (*Notifier, error) {
	if !cfg.Notification.Enabled {
		return &Notifier{enabled: false}, nil
	}

	url := strings.TrimSpace(cfg.Notification.ShoutrrURL)
	if url == "" {
		return &Notifier{enabled: false}, fmt.Errorf("notification enabled but shoutrrr_url not configured: provide URL in format 'service://credentials' (e.g., slack://token@channel, discord://token@webhookid)")
	}

	return &Notifier{
		enabled:     true,
		shoutrrrURL: cfg.Notification.ShoutrrURL,
	}, nil
}

// formatScanSummary builds the notification message from scan results.
// It is a pure function to make formatting independently testable.
func formatScanSummary(summary string, containerCount int, overall severity.Level, failed []string, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("🔍 DLIA Scan Complete\n")
	fmt.Fprintf(&sb, "🕐 Time: %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "📦 Containers: %d\n", containerCount)
	fmt.Fprintf(&sb, "Severity: %s\n", overall.Badge())

	if len(failed) > 0 {
		fmt.Fprintf(&sb, "❌ Analysis failed: %s (will be retried)\n", strings.Join(failed, ", "))
	}

	if summary != "" {
		sb.WriteString("\n")
		sb.WriteString(summary)
	}

	return sb.String()
}

// SendScanSummary delivers scan results via the configured notification channel.
func (n *Notifier) SendScanSummary(summary string, containerCount int, overall severity.Level, failed []string) error {
	if !n.enabled {
		return nil
	}

	msg := formatScanSummary(summary, containerCount, overall, failed, time.Now())

	err := shoutrrr.Send(n.shoutrrrURL, msg)
	if err != nil {
		serviceType := "unknown"
		if idx := strings.Index(n.shoutrrrURL, "://"); idx > 0 {
			serviceType = n.shoutrrrURL[:idx]
		}
		return fmt.Errorf("notification failed to send via %s (containers: %d, severity: %s): %w", serviceType, containerCount, overall, err)
	}

	return nil
}

// IsEnabled reports whether notifications are configured and active.
func (n *Notifier) IsEnabled() bool {
	return n.enabled
}

// Package notification handles sending notifications to external services.
package notification

import (
	"fmt"
	"strings"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/zorak1103/dlia/internal/config"
)

// Notifier handles sending notifications via Shoutrrr
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

// SendScanSummary delivers scan results via the configured notification channel.
func (n *Notifier) SendScanSummary(summary string, containerCount int, issuesFound bool) error {
	if !n.enabled {
		return nil // Notifications disabled
	}

	// Format the notification message
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	var sb strings.Builder
	sb.WriteString("🐳 DLIA Scan Complete\n")
	sb.WriteString(fmt.Sprintf("📅 Time: %s\n", timestamp))
	sb.WriteString(fmt.Sprintf("📦 Containers: %d\n", containerCount))

	if issuesFound {
		sb.WriteString("⚠️  Issues detected\n")
	} else {
		sb.WriteString("✅ No critical issues\n")
	}

	sb.WriteString("\n")
	sb.WriteString(summary)

	// Send notification using shoutrrr
	err := shoutrrr.Send(n.shoutrrrURL, sb.String())
	if err != nil {
		// Extract service type from URL (e.g., "slack://..." -> "slack")
		serviceType := "unknown"
		if idx := strings.Index(n.shoutrrrURL, "://"); idx > 0 {
			serviceType = n.shoutrrrURL[:idx]
		}
		return fmt.Errorf("notification failed to send via %s (containers: %d, issues: %t): %w", serviceType, containerCount, issuesFound, err)
	}

	return nil
}

// IsEnabled reports whether notifications are configured and active.
func (n *Notifier) IsEnabled() bool {
	return n.enabled
}

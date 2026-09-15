package meter

import (
	"strings"
	"time"
)

const RecentActivityWindow = 5 * time.Hour

type ActiveSubscription struct {
	Snapshot Snapshot
	Limits   []UsageWindow
}

// ActiveSubscriptions returns local subscription profiles used inside the
// lookback, preserving the collector's provider order.
func ActiveSubscriptions(dashboard Dashboard, now time.Time, lookback time.Duration) []ActiveSubscription {
	var result []ActiveSubscription
	cutoff := now.Add(-lookback)
	for _, snapshot := range dashboard.Providers {
		if snapshot.Provider != "claude" && snapshot.Provider != "codex" {
			continue
		}
		if snapshot.LastActivityAt == nil || snapshot.LastActivityAt.Before(cutoff) || snapshot.LastActivityAt.After(now) {
			continue
		}
		result = append(result, ActiveSubscription{
			Snapshot: snapshot,
			Limits:   ApplicableSubscriptionLimits(snapshot),
		})
	}
	return result
}

// ApplicableSubscriptionLimits selects the quotas that best answer "where can
// I work next?" without discarding the full provider detail in the snapshot.
func ApplicableSubscriptionLimits(snapshot Snapshot) []UsageWindow {
	var result []UsageWindow
	for _, window := range snapshot.UsageWindows {
		scope := strings.ToLower(window.Scope + " " + window.LimitID)
		fiveHour := window.WindowMinutes == 300 || window.WindowMinutes == 0 && strings.EqualFold(window.Label, "5h")
		sevenDay := window.WindowMinutes == 10080 || window.WindowMinutes == 0 && strings.EqualFold(window.Label, "7d")
		switch snapshot.Provider {
		case "claude":
			if fiveHour && (window.Scope == "" || strings.EqualFold(window.Scope, "claude")) {
				result = append(result, window)
			}
			if sevenDay && strings.Contains(scope, "fable") {
				result = append(result, window)
			}
		case "codex":
			general := window.LimitID == "codex" || window.LimitID == "" && (window.Scope == "" || strings.EqualFold(window.Scope, "codex"))
			if sevenDay && general {
				result = append(result, window)
			}
		}
	}
	return result
}

// SubscriptionSummaryLimits selects weekly limits for compact account rows.
// Claude keeps both the general and Fable weekly limits. Codex keeps only the
// general weekly limit, not a model-scoped limit such as Spark.
func SubscriptionSummaryLimits(snapshot Snapshot) []UsageWindow {
	var result []UsageWindow
	for _, window := range snapshot.UsageWindows {
		sevenDay := window.WindowMinutes == 10080 || window.WindowMinutes == 0 && strings.EqualFold(window.Label, "7d")
		if !sevenDay {
			continue
		}
		scope := strings.ToLower(window.Scope + " " + window.LimitID)
		switch snapshot.Provider {
		case "claude":
			general := window.Scope == "" || strings.EqualFold(window.Scope, "claude")
			if general || strings.Contains(scope, "fable") {
				result = append(result, window)
			}
		case "codex":
			general := window.LimitID == "codex" || window.LimitID == "" && (window.Scope == "" || strings.EqualFold(window.Scope, "codex"))
			if general {
				result = append(result, window)
			}
		default:
			result = append(result, window)
		}
	}
	return result
}

package audit

import (
	"context"
	"fmt"
	"time"
)

type SecurityDashboardMetrics struct {
	SuccessfulLogins int     `json:"successful_logins"`
	FailedLogins     int     `json:"failed_logins"`
	Lockouts         int     `json:"lockouts"`
	Anomalies        int     `json:"anomalies"`
	ActiveSessions   int     `json:"active_sessions"`
	AverageRiskScore float64 `json:"average_risk_score"`
}

type SecurityActivityPoint struct {
	Bucket           string  `json:"bucket"`
	Success          int     `json:"success"`
	Failure          int     `json:"failure"`
	AverageRiskScore float64 `json:"average_risk_score"`
}

type SecurityCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type SecurityDashboard struct {
	Range            string                   `json:"range"`
	Since            string                   `json:"since"`
	GeneratedAt      string                   `json:"generated_at"`
	Metrics          SecurityDashboardMetrics `json:"metrics"`
	AuthActivity     []SecurityActivityPoint  `json:"auth_activity"`
	AnomalyBreakdown []SecurityCount          `json:"anomaly_breakdown"`
	LoginCountries   []SecurityCount          `json:"login_countries"`
	FailedLoginIPs   []SecurityCount          `json:"failed_login_ips"`
	BlockedCountries []SecurityCount          `json:"blocked_countries"`
}

type dashboardRange struct {
	name       string
	since      time.Time
	bucketExpr string
	format     string
	count      int
	step       time.Duration
}

func parseDashboardRange(value string, now time.Time) dashboardRange {
	now = now.UTC()
	switch value {
	case "24h":
		start := now.Truncate(time.Hour).Add(-23 * time.Hour)
		return dashboardRange{
			name:       "24h",
			since:      start,
			bucketExpr: "date_trunc('hour', created_at AT TIME ZONE 'UTC')",
			format:     "2006-01-02T15:00:00Z",
			count:      24,
			step:       time.Hour,
		}
	case "30d":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -29)
		return dashboardRange{
			name:       "30d",
			since:      start,
			bucketExpr: "date_trunc('day', created_at AT TIME ZONE 'UTC')",
			format:     "2006-01-02",
			count:      30,
			step:       24 * time.Hour,
		}
	default:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -6)
		return dashboardRange{
			name:       "7d",
			since:      start,
			bucketExpr: "date_trunc('day', created_at AT TIME ZONE 'UTC')",
			format:     "2006-01-02",
			count:      7,
			step:       24 * time.Hour,
		}
	}
}

func (r *Repository) SecurityDashboard(ctx context.Context, rangeValue string) (SecurityDashboard, error) {
	now := time.Now().UTC()
	window := parseDashboardRange(rangeValue, now)
	result := SecurityDashboard{
		Range:            window.name,
		Since:            window.since.Format(time.RFC3339),
		GeneratedAt:      now.Format(time.RFC3339),
		AuthActivity:     make([]SecurityActivityPoint, 0, window.count),
		AnomalyBreakdown: []SecurityCount{},
		LoginCountries:   []SecurityCount{},
		FailedLoginIPs:   []SecurityCount{},
		BlockedCountries: []SecurityCount{},
	}

	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE action = 'auth.login_success'),
		       COUNT(*) FILTER (WHERE action = 'auth.login_failed'),
		       COUNT(*) FILTER (WHERE action = 'auth.login_failed' AND metadata->>'reason' = 'account_locked'),
		       COUNT(*) FILTER (WHERE action = 'login.anomaly'),
		       COALESCE(AVG((metadata->>'risk_score')::numeric) FILTER (WHERE action IN ('auth.login_success', 'auth.login_blocked')), 0)
		FROM audit_logs
		WHERE created_at >= $1
	`, window.since).Scan(
		&result.Metrics.SuccessfulLogins,
		&result.Metrics.FailedLogins,
		&result.Metrics.Lockouts,
		&result.Metrics.Anomalies,
		&result.Metrics.AverageRiskScore,
	); err != nil {
		return SecurityDashboard{}, fmt.Errorf("query security dashboard metrics: %w", err)
	}

	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sessions
		WHERE is_revoked = false AND expires_at > NOW()
	`).Scan(&result.Metrics.ActiveSessions); err != nil {
		return SecurityDashboard{}, fmt.Errorf("query active sessions: %w", err)
	}

	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT %s AS bucket,
		       COUNT(*) FILTER (WHERE action = 'auth.login_success'),
		       COUNT(*) FILTER (WHERE action = 'auth.login_failed'),
		       COALESCE(AVG((metadata->>'risk_score')::numeric) FILTER (WHERE action IN ('auth.login_success', 'auth.login_blocked')), 0)
		FROM audit_logs
		WHERE created_at >= $1
		  AND action IN ('auth.login_success', 'auth.login_failed', 'auth.login_blocked')
		GROUP BY bucket
		ORDER BY bucket
	`, window.bucketExpr), window.since)
	if err != nil {
		return SecurityDashboard{}, fmt.Errorf("query authentication activity: %w", err)
	}
	counts := make(map[string]SecurityActivityPoint)
	for rows.Next() {
		var bucket time.Time
		var point SecurityActivityPoint
		if err := rows.Scan(&bucket, &point.Success, &point.Failure, &point.AverageRiskScore); err != nil {
			rows.Close()
			return SecurityDashboard{}, fmt.Errorf("scan authentication activity: %w", err)
		}
		point.Bucket = bucket.UTC().Format(window.format)
		counts[point.Bucket] = point
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return SecurityDashboard{}, fmt.Errorf("read authentication activity: %w", err)
	}
	rows.Close()

	for i := 0; i < window.count; i++ {
		key := window.since.Add(time.Duration(i) * window.step).Format(window.format)
		if point, ok := counts[key]; ok {
			result.AuthActivity = append(result.AuthActivity, point)
		} else {
			result.AuthActivity = append(result.AuthActivity, SecurityActivityPoint{Bucket: key})
		}
	}

	if result.AnomalyBreakdown, err = r.securityCounts(ctx, `
		SELECT COALESCE(NULLIF(metadata->>'anomaly_type', ''), 'unknown'), COUNT(*)
		FROM audit_logs
		WHERE created_at >= $1 AND action = 'login.anomaly'
		GROUP BY 1
		ORDER BY 2 DESC, 1 ASC
	`, window.since); err != nil {
		return SecurityDashboard{}, fmt.Errorf("query anomaly breakdown: %w", err)
	}

	if result.LoginCountries, err = r.securityCounts(ctx, `
		SELECT COALESCE(NULLIF(metadata #>> '{location,country}', ''), 'Unknown'), COUNT(*)
		FROM audit_logs
		WHERE created_at >= $1 AND action = 'auth.login_success'
		GROUP BY 1
		ORDER BY 2 DESC, 1 ASC
		LIMIT 8
	`, window.since); err != nil {
		return SecurityDashboard{}, fmt.Errorf("query login countries: %w", err)
	}

	if result.FailedLoginIPs, err = r.securityCounts(ctx, `
		SELECT ip_address::text, COUNT(*)
		FROM audit_logs
		WHERE created_at >= $1 AND action = 'auth.login_failed' AND ip_address IS NOT NULL
		GROUP BY 1
		ORDER BY 2 DESC, 1 ASC
		LIMIT 8
	`, window.since); err != nil {
		return SecurityDashboard{}, fmt.Errorf("query failed login IPs: %w", err)
	}

	if result.BlockedCountries, err = r.securityCounts(ctx, `
		SELECT COALESCE(NULLIF(metadata #>> '{location,country}', ''), 'Unknown'), COUNT(*)
		FROM audit_logs
		WHERE created_at >= $1 AND action = 'auth.login_blocked'
		GROUP BY 1
		ORDER BY 2 DESC, 1 ASC
		LIMIT 8
	`, window.since); err != nil {
		return SecurityDashboard{}, fmt.Errorf("query blocked countries: %w", err)
	}

	return result, nil
}

func (r *Repository) securityCounts(ctx context.Context, query string, since time.Time) ([]SecurityCount, error) {
	rows, err := r.db.Query(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]SecurityCount, 0)
	for rows.Next() {
		var item SecurityCount
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

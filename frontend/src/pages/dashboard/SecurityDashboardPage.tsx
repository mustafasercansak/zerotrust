import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type SecurityDashboardCount, type SecurityDashboardData } from "@/lib/api";
import { useMeContext } from "@/contexts/MeContext";
import { humanizeSecurityLabel } from "./securityDashboardLabels";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";

type DashboardRange = SecurityDashboardData["range"];

const ranges: DashboardRange[] = ["24h", "7d", "30d"];

function MetricCard({ label, value, accent }: { label: string; value: number; accent: string }) {
  return (
    <Paper variant="outlined" sx={{ p: 2.5, minWidth: 0, position: "relative", overflow: "hidden" }}>
      <Box sx={{ position: "absolute", inset: "0 auto 0 0", width: 3, bgcolor: accent }} />
      <Typography variant="caption" color="text.secondary">{label}</Typography>
      <Typography variant="h4" sx={{ mt: 0.75, fontWeight: 750, letterSpacing: -0.5 }}>{value}</Typography>
    </Paper>
  );
}

function ActivityChart({ data }: { data: SecurityDashboardData["auth_activity"] }) {
  const { t, i18n } = useTranslation("securityDashboard");
  const width = 900;
  const height = 230;
  const padding = { top: 20, right: 20, bottom: 42, left: 36 };
  const chartWidth = width - padding.left - padding.right;
  const chartHeight = height - padding.top - padding.bottom;
  const maxValue = Math.max(1, ...data.flatMap((point) => [point.success, point.failure]));
  const groupWidth = chartWidth / Math.max(data.length, 1);
  const barWidth = Math.max(2, Math.min(14, groupWidth * 0.3));
  const labelEvery = data.length > 12 ? Math.ceil(data.length / 6) : 1;

  const formatBucket = (bucket: string) => {
    const date = new Date(bucket.length === 10 ? `${bucket}T00:00:00Z` : bucket);
    return new Intl.DateTimeFormat(i18n.language, data.length > 12
      ? { hour: "2-digit" }
      : { month: "short", day: "numeric" }).format(date);
  };

  return (
    <Paper variant="outlined" sx={{ p: 2.5, gridColumn: { lg: "span 2" } }}>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 2, mb: 1 }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>{t("activityTitle")}</Typography>
        <Box sx={{ display: "flex", gap: 2 }}>
          {[
            ["#22c55e", t("successful")],
            ["#f43f5e", t("failed")],
          ].map(([color, label]) => (
            <Box key={label} sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
              <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: color }} />
              <Typography variant="caption" color="text.secondary">{label}</Typography>
            </Box>
          ))}
        </Box>
      </Box>
      <Box sx={{ overflowX: "auto" }}>
        <svg viewBox={`0 0 ${width} ${height}`} width="100%" height={height} style={{ minWidth: 620, display: "block" }} role="img" aria-label={t("activityTitle")}>
          {[0, 0.5, 1].map((ratio) => {
            const y = padding.top + chartHeight * ratio;
            return <line key={ratio} x1={padding.left} y1={y} x2={width - padding.right} y2={y} stroke="currentColor" opacity="0.08" />;
          })}
          {data.map((point, index) => {
            const center = padding.left + groupWidth * index + groupWidth / 2;
            const successHeight = (point.success / maxValue) * chartHeight;
            const failureHeight = (point.failure / maxValue) * chartHeight;
            return (
              <g key={point.bucket}>
                <rect x={center - barWidth - 1} y={padding.top + chartHeight - successHeight} width={barWidth} height={successHeight} rx="2" fill="#22c55e" />
                <rect x={center + 1} y={padding.top + chartHeight - failureHeight} width={barWidth} height={failureHeight} rx="2" fill="#f43f5e" />
                {index % labelEvery === 0 && (
                  <text x={center} y={height - 12} textAnchor="middle" fill="currentColor" opacity="0.55" fontSize="11">
                    {formatBucket(point.bucket)}
                  </text>
                )}
              </g>
            );
          })}
        </svg>
      </Box>
    </Paper>
  );
}

function RankingCard({
  title,
  items,
  empty,
  labelFor = (name) => name,
}: {
  title: string;
  items: SecurityDashboardCount[];
  empty: string;
  labelFor?: (name: string) => string;
}) {
  const maxCount = Math.max(1, ...items.map((item) => item.count));
  return (
    <Paper variant="outlined" sx={{ p: 2.5, minHeight: 250 }}>
      <Typography variant="subtitle1" sx={{ fontWeight: 700, mb: 2 }}>{title}</Typography>
      {items.length === 0 ? (
        <Typography variant="body2" color="text.secondary">{empty}</Typography>
      ) : (
        <Box sx={{ display: "grid", gap: 1.75 }}>
          {items.map((item) => {
            const label = labelFor(item.name);
            return (
            <Box key={item.name}>
              <Box sx={{ display: "flex", justifyContent: "space-between", gap: 2, mb: 0.5 }}>
                <Typography variant="body2" noWrap title={label}>{label}</Typography>
                <Typography variant="body2" sx={{ fontWeight: 700 }}>{item.count}</Typography>
              </Box>
              <Box sx={{ height: 5, borderRadius: 4, bgcolor: "action.hover", overflow: "hidden" }}>
                <Box sx={{ width: `${(item.count / maxCount) * 100}%`, height: "100%", bgcolor: "primary.main", borderRadius: 4 }} />
              </Box>
            </Box>
            );
          })}
        </Box>
      )}
    </Paper>
  );
}

export default function SecurityDashboardPage() {
  const { t } = useTranslation("securityDashboard");
  const me = useMeContext();
  const [range, setRange] = useState<DashboardRange>("7d");
  const [data, setData] = useState<SecurityDashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api.securityDashboard(range)
      .then((result) => {
        if (!cancelled) setData(result);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [range]);

  const metrics = useMemo(() => data ? [
    { label: t("metrics.successfulLogins"), value: data.metrics.successful_logins, accent: "#22c55e" },
    { label: t("metrics.failedLogins"), value: data.metrics.failed_logins, accent: "#f43f5e" },
    { label: t("metrics.lockouts"), value: data.metrics.lockouts, accent: "#f59e0b" },
    { label: t("metrics.anomalies"), value: data.metrics.anomalies, accent: "#a855f7" },
    { label: t("metrics.activeSessions"), value: data.metrics.active_sessions, accent: "#3b82f6" },
  ] : [], [data, t]);
  const anomalyLabel = (name: string) => t(`anomalyTypes.${name}`, {
    defaultValue: humanizeSecurityLabel(name),
  });
  const countryLabel = (name: string) => name.toLowerCase() === "unknown"
    ? t("locations.unknown")
    : name;

  if (!me?.roles.includes("admin")) {
    return <Box sx={{ p: 4 }}><Alert severity="error">{t("accessDenied")}</Alert></Box>;
  }

  return (
    <Box sx={{ height: "100%", overflow: "auto", p: { xs: 2, md: 4 } }}>
      <Box sx={{ display: "flex", alignItems: { xs: "flex-start", sm: "center" }, justifyContent: "space-between", gap: 2, flexDirection: { xs: "column", sm: "row" }, mb: 3 }}>
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 750 }}>{t("title")}</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{t("subtitle")}</Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 0.75 }}>
          {ranges.map((value) => (
            <Button key={value} size="small" variant={range === value ? "contained" : "outlined"} onClick={() => setRange(value)}>
              {t(`ranges.${value}`)}
            </Button>
          ))}
        </Box>
      </Box>

      {error && <Alert severity="error" sx={{ mb: 2 }}>{t("loadError")}</Alert>}
      {loading && !data ? (
        <Box sx={{ height: 360, display: "grid", placeItems: "center" }}><CircularProgress size={24} /></Box>
      ) : data && (
        <>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", xl: "repeat(5, 1fr)" }, gap: 2, mb: 2 }}>
            {metrics.map((metric) => <MetricCard key={metric.label} {...metric} />)}
          </Box>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "repeat(2, minmax(0, 1fr))" }, gap: 2 }}>
            <ActivityChart data={data.auth_activity} />
            <RankingCard title={t("anomaliesTitle")} items={data.anomaly_breakdown} empty={t("noData")} labelFor={anomalyLabel} />
            <RankingCard title={t("countriesTitle")} items={data.login_countries} empty={t("noData")} labelFor={countryLabel} />
            <RankingCard title={t("failedIPsTitle")} items={data.failed_login_ips} empty={t("noData")} />
          </Box>
        </>
      )}
    </Box>
  );
}

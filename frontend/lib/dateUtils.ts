const DATE_OPTS: Intl.DateTimeFormatOptions = {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
};

const DATETIME_OPTS: Intl.DateTimeFormatOptions = {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
};

export function formatDate(
  iso: string | null | undefined,
  locale: string,
  fallback = "—",
): string {
  if (!iso) return fallback;
  const d = new Date(iso);
  if (isNaN(d.getTime())) return fallback;
  return new Intl.DateTimeFormat(locale, DATE_OPTS).format(d);
}

export function formatDateTime(
  iso: string | null | undefined,
  locale: string,
  fallback = "—",
): string {
  if (!iso) return fallback;
  const d = new Date(iso);
  if (isNaN(d.getTime())) return fallback;
  return new Intl.DateTimeFormat(locale, DATETIME_OPTS).format(d);
}

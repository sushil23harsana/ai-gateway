// Server-side gateway client. Holds ADMIN_TOKEN; only imported by server
// components and route handlers, so the token never reaches the browser.
import type {
  Overview,
  TimeBucket,
  ModelStat,
  ProviderStat,
  KeyStat,
  KeyRecord,
  Live,
  RecentRequest,
  CacheStats,
} from "./types";

const BASE = process.env.GATEWAY_URL ?? "http://localhost:8080";
const TOKEN = process.env.ADMIN_TOKEN ?? "change-me";

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
    cache: "no-store",
  });
  if (!res.ok) throw new Error(`gateway ${path} -> HTTP ${res.status}`);
  return (await res.json()) as T;
}

// safe wraps a fetch so one failing endpoint doesn't blank the whole page.
async function safe<T>(p: Promise<T>, fallback: T): Promise<T> {
  try {
    return await p;
  } catch {
    return fallback;
  }
}

export const getOverview = () =>
  safe(get<Overview>("/admin/stats/overview"), {
    spend_today_usd: 0,
    spend_month_usd: 0,
    total_requests: 0,
    cache_hit_rate: 0,
    latency_p50_ms: 0,
    latency_p95_ms: 0,
    total_tokens_in: 0,
    total_tokens_out: 0,
  });

export const getTimeseries = (range = "24h") =>
  safe(get<{ range: string; buckets: TimeBucket[] }>(`/admin/stats/timeseries?range=${range}`), {
    range,
    buckets: [],
  });

export const getByModel = () =>
  safe(get<{ models: ModelStat[] }>("/admin/stats/by-model"), { models: [] });

export const getByProvider = () =>
  safe(get<{ providers: ProviderStat[] }>("/admin/stats/by-provider"), { providers: [] });

export const getByKey = () => safe(get<{ keys: KeyStat[] }>("/admin/stats/by-key"), { keys: [] });

// getKeys returns the management list (settings) from the control plane.
export const getKeys = () => safe(get<{ keys: KeyRecord[] }>("/admin/keys"), { keys: [] });

// gatewayWrite forwards a control-plane mutation to the gateway with the admin
// token attached. It is only ever called from server-side route handlers, so the
// token never reaches the browser. The raw status/body are passed straight back.
export async function gatewayWrite(
  method: "POST" | "PATCH" | "DELETE",
  path: string,
  body?: unknown,
): Promise<{ status: number; body: string; contentType: string }> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: { Authorization: `Bearer ${TOKEN}`, "Content-Type": "application/json" },
    cache: "no-store",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  return {
    status: res.status,
    body: await res.text(),
    contentType: res.headers.get("content-type") ?? "application/json",
  };
}

// writesEnabled reports whether a real admin token is configured. The gateway's
// WriteGuard enforces this server-side; the dashboard uses it only to disable
// controls and show a hint instead of surfacing a 403 after the fact.
export const writesEnabled = (): boolean => TOKEN !== "" && TOKEN !== "change-me";

export const getRecent = (limit = 60) =>
  safe(get<{ requests: RecentRequest[] }>(`/admin/stats/recent?limit=${limit}`), { requests: [] });

export const getCache = () =>
  safe(get<CacheStats>("/admin/stats/cache"), { semantic_entries: 0, recent_semantic: [] });

// probe returns whether the gateway answered (used to show a connection banner).
export async function probe(): Promise<boolean> {
  try {
    await get<Overview>("/admin/stats/overview");
    return true;
  } catch {
    return false;
  }
}

export type { Live };

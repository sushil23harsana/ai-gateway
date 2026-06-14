// Mirrors the gateway's /admin/stats/* JSON shapes (internal/store/stats.go).

export type Overview = {
  spend_today_usd: number;
  spend_month_usd: number;
  total_requests: number;
  cache_hit_rate: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  total_tokens_in: number;
  total_tokens_out: number;
};

export type TimeBucket = { timestamp: string; requests: number; cost_usd: number };
export type ModelStat = {
  model: string;
  provider: string;
  requests: number;
  tokens_in: number;
  tokens_out: number;
  cost_usd: number;
};
export type ProviderStat = { provider: string; requests: number; cost_usd: number };
export type KeyStat = {
  id: string;
  name: string;
  rate_limit_rpm: number;
  monthly_budget_usd?: number;
  disabled: boolean;
  requests: number;
  month_cost_usd: number;
  total_cost_usd: number;
};
export type MinuteCount = { timestamp: number; count: number };
export type Live = { current_per_minute: number; recent: MinuteCount[] };

import { DollarSign, ArrowLeftRight, Zap, Gauge, Info } from "lucide-react";
import { getByKey, getByModel, getByProvider, getOverview, getTimeseries, probe } from "@/lib/api";
import { compact, ms, num, pct, usd } from "@/lib/format";
import AutoRefresh from "@/components/AutoRefresh";
import Badge from "@/components/Badge";
import ByProviderChart from "@/components/ByProviderChart";
import Card from "@/components/Card";
import KeysTable from "@/components/KeysTable";
import LiveTile from "@/components/LiveTile";
import ModelTable from "@/components/ModelTable";
import ProgressBar from "@/components/ProgressBar";
import Sidebar from "@/components/Sidebar";
import SpendChart from "@/components/SpendChart";
import StatCard from "@/components/StatCard";
import Topbar from "@/components/Topbar";

export const dynamic = "force-dynamic";

function sectionHead(title: string, note?: string) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "18px 22px" }}>
      <span style={{ fontSize: 16, fontWeight: 700 }}>{title}</span>
      {note ? (
        <Badge tone="neutral" variant="outline">
          {note}
        </Badge>
      ) : null}
    </div>
  );
}

export default async function Page({ searchParams }: { searchParams?: { range?: string } }) {
  const range = searchParams?.range === "7d" || searchParams?.range === "30d" ? searchParams.range : "24h";

  const [connected, overview, ts, byModel, byProvider, byKey] = await Promise.all([
    probe(),
    getOverview(),
    getTimeseries(range),
    getByModel(),
    getByProvider(),
    getByKey(),
  ]);

  const providerCount = byProvider.providers.length;
  const modelCount = byModel.models.length;
  const totalReq = byProvider.providers.reduce((s, p) => s + p.requests, 0) || 1;
  const topProviderShare = byProvider.providers.length ? (byProvider.providers[0].requests / totalReq) * 100 : 0;
  const budgetedKey = byKey.keys.find((k) => k.monthly_budget_usd);
  const topKeyBudgetPct = budgetedKey
    ? Math.min(100, (budgetedKey.month_cost_usd / (budgetedKey.monthly_budget_usd || 1)) * 100)
    : null;

  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw", background: "var(--bg-base)", overflow: "hidden" }}>
      <Sidebar />
      <main style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--bg-canvas)" }}>
        <Topbar
          title="Gateway Overview"
          subtitle="Real-time routing, cost & cache across your providers"
          range={range}
          connected={connected}
        />
        <div style={{ flex: 1, overflowY: "auto", padding: 28 }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 1440, margin: "0 auto" }}>
            {!connected && (
              <div
                style={{
                  border: "1px solid rgba(255,92,122,0.35)",
                  background: "var(--negative-bg)",
                  color: "var(--negative)",
                  fontSize: 13,
                  padding: "11px 16px",
                  borderRadius: "var(--radius-md)",
                }}
              >
                Can&rsquo;t reach the gateway admin API — check GATEWAY_URL / ADMIN_TOKEN. Showing zeros.
              </div>
            )}

            {/* Hero stats */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16 }}>
              <StatCard label="Spend · month" value={usd(overview.spend_month_usd)} icon={<DollarSign size={16} />} footnote={`today ${usd(overview.spend_today_usd)}`} accentValue delay={0} />
              <StatCard label="Requests" value={num(overview.total_requests)} icon={<ArrowLeftRight size={16} />} footnote={`${compact(overview.total_tokens_in)} in · ${compact(overview.total_tokens_out)} out`} delay={40} />
              <StatCard label="Cache hit rate" value={pct(overview.cache_hit_rate)} icon={<Zap size={16} />} footnote="exact + semantic" accentValue delay={80} />
              <StatCard label="P95 latency" value={ms(overview.latency_p95_ms)} icon={<Gauge size={16} />} footnote={`p50 ${ms(overview.latency_p50_ms)} · non-cache`} delay={120} />
            </div>

            {/* Smart Router mesh card + cache performance */}
            <div style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr", gap: 16 }}>
              <Card mesh padding="28px" style={{ minHeight: 220 }}>
                <div style={{ display: "flex", flexDirection: "column", height: "100%", justifyContent: "space-between", gap: 18 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <Badge tone="violet" variant="solid">
                      LIVE
                    </Badge>
                    <span className="eyebrow" style={{ color: "var(--violet-200)" }}>
                      Smart Router
                    </span>
                  </div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                    <span style={{ color: "#fff", fontSize: 30, fontWeight: 700, letterSpacing: "-0.01em", lineHeight: 1.1 }}>
                      {modelCount} model{modelCount === 1 ? "" : "s"} across {providerCount} provider{providerCount === 1 ? "" : "s"}
                    </span>
                    <span style={{ color: "rgba(255,255,255,0.66)", fontSize: 14, maxWidth: 460 }}>
                      Each request is routed by model with cross-provider failover. Identical and near-duplicate prompts are
                      served from cache, skipping the provider entirely.
                    </span>
                  </div>
                  <div style={{ display: "flex", gap: 26, marginTop: 4 }}>
                    {[
                      ["Cache hit rate", pct(overview.cache_hit_rate)],
                      ["Requests", num(overview.total_requests)],
                      ["p50 latency", ms(overview.latency_p50_ms)],
                    ].map(([k, v]) => (
                      <div key={k} style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                        <span className="mono" style={{ color: "#fff", fontSize: 22, fontWeight: 700 }}>
                          {v}
                        </span>
                        <span style={{ color: "rgba(255,255,255,0.55)", fontSize: 11 }}>{k}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </Card>

              <Card padding="22px">
                <div style={{ display: "flex", flexDirection: "column", gap: 16, height: "100%" }}>
                  <span className="eyebrow">Cache &amp; usage</span>
                  <ProgressBar value={overview.cache_hit_rate * 100} tone="positive" label="Cache hit rate" showValue />
                  <ProgressBar value={topProviderShare} tone="violet" label="Top provider share" showValue />
                  {topKeyBudgetPct != null && (
                    <ProgressBar
                      value={topKeyBudgetPct}
                      tone={topKeyBudgetPct >= 100 ? "negative" : topKeyBudgetPct >= 80 ? "warning" : "positive"}
                      label={`Budget · ${budgetedKey?.name}`}
                      showValue
                    />
                  )}
                  <div style={{ marginTop: "auto", display: "flex", alignItems: "center", gap: 8, color: "var(--text-secondary)", fontSize: 12 }}>
                    <Info size={14} /> <span>Exact + semantic caching · hits served in &lt;10ms</span>
                  </div>
                </div>
              </Card>
            </div>

            {/* Spend chart + live throughput */}
            <div style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr", gap: 16 }}>
              <Card padding="0">
                {sectionHead("Spend & requests", range)}
                <div style={{ padding: "0 20px 18px" }}>
                  <SpendChart buckets={ts.buckets} range={ts.range} />
                </div>
              </Card>
              <Card padding="22px">
                <span className="eyebrow">Live · throughput</span>
                <div style={{ marginTop: 14 }}>
                  <LiveTile />
                </div>
              </Card>
            </div>

            {/* Provider split + model distribution */}
            <div style={{ display: "grid", gridTemplateColumns: "1fr 2.4fr", gap: 16 }}>
              <Card padding="22px">
                <span className="eyebrow">Requests by provider</span>
                <div style={{ marginTop: 14 }}>
                  <ByProviderChart providers={byProvider.providers} />
                </div>
              </Card>
              <Card padding="0">
                {sectionHead("Model distribution", `last ${range}`)}
                <ModelTable models={byModel.models} />
              </Card>
            </div>

            {/* Virtual keys */}
            <Card padding="0">
              {sectionHead("Virtual keys · spend vs budget")}
              <KeysTable keys={byKey.keys} />
            </Card>

            <div style={{ display: "flex", justifyContent: "space-between", color: "var(--text-tertiary)", fontSize: 11, paddingTop: 4 }}>
              <span className="mono">gateway /admin/stats/* · auto-refresh 20s</span>
              <span className="mono">ai-gateway console</span>
            </div>
          </div>
          <AutoRefresh intervalMs={20000} />
        </div>
      </main>
    </div>
  );
}

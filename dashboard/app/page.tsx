import { getByKey, getByModel, getByProvider, getOverview, getTimeseries, probe } from "@/lib/api";
import { compact, ms, num, pct, usd } from "@/lib/format";
import AutoRefresh from "@/components/AutoRefresh";
import ByModelChart from "@/components/ByModelChart";
import ByProviderChart from "@/components/ByProviderChart";
import KeysTable from "@/components/KeysTable";
import LiveTile from "@/components/LiveTile";
import Panel from "@/components/Panel";
import SpendChart from "@/components/SpendChart";
import Stat from "@/components/Stat";

export const dynamic = "force-dynamic";

export default async function Page() {
  const [connected, overview, ts, byModel, byProvider, byKey] = await Promise.all([
    probe(),
    getOverview(),
    getTimeseries("24h"),
    getByModel(),
    getByProvider(),
    getByKey(),
  ]);

  return (
    <main className="shell">
      <header className="topbar">
        <div className="brand">
          <span className="blip" />
          ai-gateway
          <span className="brand-sub">/ console</span>
        </div>
        <div className="topbar-right">
          <span className="status">
            <span className={`dot${connected ? "" : " down"}`} />
            {connected ? "gateway online" : "gateway offline"}
          </span>
        </div>
      </header>

      {!connected ? (
        <div className="banner">
          Can&rsquo;t reach the gateway at its admin API. Check GATEWAY_URL / ADMIN_TOKEN — showing last-known
          zeros.
        </div>
      ) : null}

      <section className="stats">
        <Stat label="Spend · today" value={usd(overview.spend_today_usd)} sub="since 00:00 UTC" delay={0} />
        <Stat label="Spend · month" value={usd(overview.spend_month_usd)} sub="month to date" accent delay={40} />
        <Stat
          label="Requests"
          value={num(overview.total_requests)}
          sub={`${compact(overview.total_tokens_in)} in · ${compact(overview.total_tokens_out)} out`}
          delay={80}
        />
        <Stat label="Cache hit rate" value={pct(overview.cache_hit_rate)} sub="of all requests" accent delay={120} />
        <Stat label="Latency · p50" value={ms(overview.latency_p50_ms)} sub="median (non-cache)" delay={160} />
        <Stat label="Latency · p95" value={ms(overview.latency_p95_ms)} sub="tail (non-cache)" delay={200} />
      </section>

      <section className="grid-2">
        <Panel title="Spend & requests · last 24h" note="hourly">
          <SpendChart buckets={ts.buckets} range={ts.range} />
        </Panel>
        <Panel title="Requests by provider">
          <ByProviderChart providers={byProvider.providers} />
        </Panel>
      </section>

      <section className="grid-2 even">
        <Panel title="Cost by model" note="top 8">
          <ByModelChart models={byModel.models} />
        </Panel>
        <Panel title="Live · throughput">
          <LiveTile />
        </Panel>
      </section>

      <Panel title="Virtual keys · spend vs budget">
        <KeysTable keys={byKey.keys} />
      </Panel>

      <footer className="foot">
        <span>data · gateway /admin/stats/* · auto-refresh 20s</span>
        <span>ai-gateway console</span>
      </footer>

      <AutoRefresh intervalMs={20000} />
    </main>
  );
}

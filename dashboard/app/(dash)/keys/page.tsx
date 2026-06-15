import { getByKey, getKeys, probe, writesEnabled } from "@/lib/api";
import { num, usd } from "@/lib/format";
import AutoRefresh from "@/components/AutoRefresh";
import Card from "@/components/Card";
import KeysManager, { type KeyRow } from "@/components/KeysManager";
import StatCard from "@/components/StatCard";
import Topbar from "@/components/Topbar";

export const dynamic = "force-dynamic";

export default async function KeysPage() {
  // Management list (settings) is authoritative for the rows; per-key spend is
  // merged in from the stats aggregate by id.
  const [connected, mgmt, byKey] = await Promise.all([probe(), getKeys(), getByKey()]);
  const stats = new Map(byKey.keys.map((k) => [k.id, k]));
  const rows: KeyRow[] = mgmt.keys.map((k) => {
    const s = stats.get(k.id);
    return {
      ...k,
      month_cost_usd: s?.month_cost_usd ?? 0,
      total_cost_usd: s?.total_cost_usd ?? 0,
      requests: s?.requests ?? 0,
    };
  });

  const active = rows.filter((k) => !k.disabled).length;
  const totalMonth = rows.reduce((s, k) => s + k.month_cost_usd, 0);
  const totalReq = rows.reduce((s, k) => s + k.requests, 0);

  return (
    <>
      <Topbar title="API Keys" subtitle="Virtual keys, budgets & per-key spend" connected={connected} />
      <div style={{ flex: 1, overflowY: "auto", padding: 28 }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 1440, margin: "0 auto" }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16 }}>
            <StatCard label="Keys" value={String(rows.length)} footnote={`${active} active`} accentValue />
            <StatCard label="Spend · month" value={usd(totalMonth)} footnote="across all keys" />
            <StatCard label="Requests" value={num(totalReq)} footnote="all keys" />
          </div>
          <Card padding="0">
            <KeysManager initialKeys={rows} writesEnabled={writesEnabled()} />
          </Card>
        </div>
        <AutoRefresh intervalMs={20000} />
      </div>
    </>
  );
}

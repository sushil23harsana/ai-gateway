import { getByKey, probe } from "@/lib/api";
import { num, usd } from "@/lib/format";
import AutoRefresh from "@/components/AutoRefresh";
import Card from "@/components/Card";
import KeysTable from "@/components/KeysTable";
import SectionHead from "@/components/SectionHead";
import StatCard from "@/components/StatCard";
import Topbar from "@/components/Topbar";

export const dynamic = "force-dynamic";

export default async function KeysPage() {
  const [connected, byKey] = await Promise.all([probe(), getByKey()]);
  const keys = byKey.keys;
  const active = keys.filter((k) => !k.disabled).length;
  const totalMonth = keys.reduce((s, k) => s + k.month_cost_usd, 0);
  const totalReq = keys.reduce((s, k) => s + k.requests, 0);

  return (
    <>
      <Topbar title="API Keys" subtitle="Virtual keys, budgets & per-key spend" connected={connected} />
      <div style={{ flex: 1, overflowY: "auto", padding: 28 }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 1440, margin: "0 auto" }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16 }}>
            <StatCard label="Keys" value={String(keys.length)} footnote={`${active} active`} accentValue />
            <StatCard label="Spend · month" value={usd(totalMonth)} footnote="across all keys" />
            <StatCard label="Requests" value={num(totalReq)} footnote="all keys" />
          </div>
          <Card padding="0">
            <SectionHead title="Virtual keys · spend vs budget" />
            <KeysTable keys={keys} />
          </Card>
          <div style={{ color: "var(--text-tertiary)", fontSize: 11 }} className="mono">
            mint a key: POST /admin/keys (returns the raw key once)
          </div>
        </div>
        <AutoRefresh intervalMs={20000} />
      </div>
    </>
  );
}

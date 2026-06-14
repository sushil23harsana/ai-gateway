import { getRecent, probe } from "@/lib/api";
import AutoRefresh from "@/components/AutoRefresh";
import Card from "@/components/Card";
import LogsTable from "@/components/LogsTable";
import SectionHead from "@/components/SectionHead";
import Topbar from "@/components/Topbar";

export const dynamic = "force-dynamic";

export default async function LogsPage() {
  const [connected, recent] = await Promise.all([probe(), getRecent(60)]);

  return (
    <>
      <Topbar title="Live Logs" subtitle="Every request flowing through the gateway" connected={connected} />
      <div style={{ flex: 1, overflowY: "auto", padding: 28 }}>
        <div style={{ maxWidth: 1440, margin: "0 auto" }}>
          <Card padding="0">
            <SectionHead title="Recent requests" note={`${recent.requests.length} shown`} />
            <LogsTable rows={recent.requests} />
          </Card>
          <div style={{ color: "var(--text-tertiary)", fontSize: 11, paddingTop: 12 }} className="mono">
            gateway /admin/stats/recent · auto-refresh 10s
          </div>
        </div>
        <AutoRefresh intervalMs={10000} />
      </div>
    </>
  );
}

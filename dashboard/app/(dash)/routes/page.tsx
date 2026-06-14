import { Info } from "lucide-react";
import { getByModel, getByProvider, probe } from "@/lib/api";
import AutoRefresh from "@/components/AutoRefresh";
import Card from "@/components/Card";
import ModelTable from "@/components/ModelTable";
import SectionHead from "@/components/SectionHead";
import Topbar from "@/components/Topbar";

export const dynamic = "force-dynamic";

export default async function RoutesPage() {
  const [connected, byModel, byProvider] = await Promise.all([probe(), getByModel(), getByProvider()]);

  return (
    <>
      <Topbar title="Routing" subtitle="How requests map to providers & models" connected={connected} />
      <div style={{ flex: 1, overflowY: "auto", padding: 28 }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 1440, margin: "0 auto" }}>
          <Card padding="22px">
            <div style={{ display: "flex", gap: 12, alignItems: "flex-start" }}>
              <span style={{ color: "var(--violet-300)", marginTop: 2 }}>
                <Info size={18} />
              </span>
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                <span style={{ fontSize: 15, fontWeight: 600 }}>How routing works</span>
                <span style={{ color: "var(--text-secondary)", fontSize: 13, lineHeight: 1.6, maxWidth: 760 }}>
                  Each request is routed by its <b style={{ color: "var(--text-primary)" }}>model</b>: first the pricing
                  table&rsquo;s provider mapping, then a name-prefix heuristic (<code>claude*</code> → Anthropic,{" "}
                  <code>gpt*</code>/<code>o*</code> → OpenAI), then the default provider. On a primary{" "}
                  <b style={{ color: "var(--text-primary)" }}>5xx or timeout</b>, the gateway fails over to a configured
                  fallback provider &amp; model — both attempts are logged.
                </span>
              </div>
            </div>
          </Card>

          <Card padding="0">
            <SectionHead title="Model → provider distribution" note={`${byProvider.providers.length} providers`} />
            <ModelTable models={byModel.models} />
          </Card>
        </div>
        <AutoRefresh intervalMs={30000} />
      </div>
    </>
  );
}

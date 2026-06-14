import { Info } from "lucide-react";
import { getCache, getOverview, probe } from "@/lib/api";
import { num, pct } from "@/lib/format";
import AutoRefresh from "@/components/AutoRefresh";
import Badge from "@/components/Badge";
import Card from "@/components/Card";
import ProgressBar from "@/components/ProgressBar";
import SectionHead from "@/components/SectionHead";
import StatCard from "@/components/StatCard";
import Topbar from "@/components/Topbar";

export const dynamic = "force-dynamic";

const th: React.CSSProperties = { textAlign: "left", padding: "10px 22px", fontSize: 11, fontWeight: 600, letterSpacing: "0.06em", textTransform: "uppercase", color: "var(--text-secondary)" };
const td: React.CSSProperties = { padding: "12px 22px", fontSize: 13, borderTop: "1px solid var(--border-subtle)" };
const r: React.CSSProperties = { textAlign: "right" };

export default async function CachesPage() {
  const [connected, overview, cache] = await Promise.all([probe(), getOverview(), getCache()]);

  return (
    <>
      <Topbar title="Caches" subtitle="Exact-match + semantic response caches" connected={connected} />
      <div style={{ flex: 1, overflowY: "auto", padding: 28 }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 1440, margin: "0 auto" }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16 }}>
            <StatCard label="Cache hit rate" value={pct(overview.cache_hit_rate)} footnote="exact + semantic" accentValue />
            <StatCard label="Semantic entries" value={num(cache.semantic_entries)} footnote="stored embeddings (pgvector)" />
            <StatCard label="Requests" value={num(overview.total_requests)} footnote="all-time" />
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "1fr 1.6fr", gap: 16 }}>
            <Card padding="22px">
              <div style={{ display: "flex", flexDirection: "column", gap: 16, height: "100%" }}>
                <span className="eyebrow">Cache performance</span>
                <ProgressBar value={overview.cache_hit_rate * 100} tone="positive" label="Hit rate" showValue />
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                  <Badge tone="info" variant="soft">
                    exact-match · Redis
                  </Badge>
                  <Badge tone="violet" variant="soft">
                    semantic · pgvector
                  </Badge>
                </div>
                <div style={{ marginTop: "auto", display: "flex", alignItems: "center", gap: 8, color: "var(--text-secondary)", fontSize: 12 }}>
                  <Info size={14} /> <span>Exact hits in &lt;10ms; semantic hits embed + vector-search, still skipping the LLM.</span>
                </div>
              </div>
            </Card>

            <Card padding="0">
              <SectionHead title="Recent semantic entries" note={`${cache.recent_semantic.length} shown`} />
              {cache.recent_semantic.length ? (
                <table style={{ width: "100%", borderCollapse: "collapse" }}>
                  <thead>
                    <tr>
                      <th style={th}>Stored</th>
                      <th style={th}>Model</th>
                      <th style={{ ...th, ...r }}>Tokens</th>
                    </tr>
                  </thead>
                  <tbody>
                    {cache.recent_semantic.map((e, i) => (
                      <tr key={i}>
                        <td className="mono" style={{ ...td, color: "var(--text-secondary)" }}>
                          {new Date(e.created_at).toLocaleTimeString("en-US", { hour12: false })}
                        </td>
                        <td style={td}>
                          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                            <span style={{ fontWeight: 500 }}>{e.model}</span>
                            <Badge tone="neutral" variant="outline">
                              {e.provider}
                            </Badge>
                          </div>
                        </td>
                        <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
                          {num(e.tokens_in)}/{num(e.tokens_out)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <div style={{ color: "var(--text-tertiary)", fontSize: 13, padding: "28px 22px" }}>
                  no semantic entries yet — enable SEMANTIC_CACHE_ENABLED and send a few requests
                </div>
              )}
            </Card>
          </div>
        </div>
        <AutoRefresh intervalMs={20000} />
      </div>
    </>
  );
}

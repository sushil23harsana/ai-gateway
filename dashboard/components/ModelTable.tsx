import type { ModelStat } from "@/lib/types";
import { num, usd } from "@/lib/format";
import Badge from "./Badge";
import ProgressBar from "./ProgressBar";

const th: React.CSSProperties = {
  textAlign: "left",
  padding: "10px 22px",
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: "0.06em",
  textTransform: "uppercase",
  color: "var(--text-secondary)",
};
const td: React.CSSProperties = {
  padding: "14px 22px",
  fontSize: 14,
  color: "var(--text-primary)",
  borderTop: "1px solid var(--border-subtle)",
};
const r: React.CSSProperties = { textAlign: "right" };

export default function ModelTable({ models }: { models: ModelStat[] }) {
  if (!models.length) {
    return <div style={{ color: "var(--text-tertiary)", fontSize: 13, padding: "28px 22px" }}>no model usage yet</div>;
  }
  const total = models.reduce((s, m) => s + m.requests, 0) || 1;

  return (
    <table style={{ width: "100%", borderCollapse: "collapse" }}>
      <thead>
        <tr>
          <th style={th}>Model</th>
          <th style={{ ...th, width: "26%" }}>Share</th>
          <th style={{ ...th, ...r }}>Cost</th>
          <th style={{ ...th, ...r }}>Tokens</th>
          <th style={{ ...th, ...r }}>Requests</th>
          <th style={{ ...th, ...r }}>Status</th>
        </tr>
      </thead>
      <tbody>
        {models.map((m) => (
          <tr key={m.model + m.provider}>
            <td style={td}>
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <span style={{ width: 7, height: 7, borderRadius: "50%", background: "var(--violet-400)", boxShadow: "var(--glow-violet-sm)" }} />
                <span style={{ fontWeight: 600 }}>{m.model}</span>
                <Badge tone="neutral" variant="outline">
                  {m.provider}
                </Badge>
              </div>
            </td>
            <td style={td}>
              <ProgressBar value={(m.requests / total) * 100} height={6} glow={false} />
            </td>
            <td className="mono" style={{ ...td, ...r }}>
              {usd(m.cost_usd)}
            </td>
            <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
              {num(m.tokens_in + m.tokens_out)}
            </td>
            <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
              {num(m.requests)}
            </td>
            <td style={{ ...td, ...r }}>
              <Badge tone="positive" dot>
                healthy
              </Badge>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

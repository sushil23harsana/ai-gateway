import type { RecentRequest } from "@/lib/types";
import { num, usd } from "@/lib/format";
import Badge from "./Badge";

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
  padding: "12px 22px",
  fontSize: 13,
  color: "var(--text-primary)",
  borderTop: "1px solid var(--border-subtle)",
};
const r: React.CSSProperties = { textAlign: "right" };

function time(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", { hour12: false });
}

function StateBadge({ row }: { row: RecentRequest }) {
  if (row.status >= 400) return <Badge tone="negative" dot>{`error ${row.status}`}</Badge>;
  if (row.cache_hit) return <Badge tone="info" dot>cached</Badge>;
  return <Badge tone="neutral" dot>live</Badge>;
}

export default function LogsTable({ rows }: { rows: RecentRequest[] }) {
  if (!rows.length) {
    return <div style={{ color: "var(--text-tertiary)", fontSize: 13, padding: "28px 22px" }}>no requests yet</div>;
  }
  return (
    <table style={{ width: "100%", borderCollapse: "collapse" }}>
      <thead>
        <tr>
          <th style={th}>Time</th>
          <th style={th}>Key</th>
          <th style={th}>Model</th>
          <th style={{ ...th, ...r }}>Tokens</th>
          <th style={{ ...th, ...r }}>Cost</th>
          <th style={{ ...th, ...r }}>Latency</th>
          <th style={{ ...th, ...r }}>State</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row, i) => (
          <tr key={i}>
            <td className="mono" style={{ ...td, color: "var(--text-secondary)" }}>
              {time(row.created_at)}
            </td>
            <td style={td}>{row.key_name ?? <span style={{ color: "var(--text-tertiary)" }}>—</span>}</td>
            <td style={td}>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ fontWeight: 500 }}>{row.model}</span>
                <Badge tone="neutral" variant="outline">
                  {row.provider}
                </Badge>
              </div>
            </td>
            <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
              {num(row.tokens_in)}/{num(row.tokens_out)}
            </td>
            <td className="mono" style={{ ...td, ...r }}>
              {usd(row.cost_usd)}
            </td>
            <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
              {num(row.latency_ms)} ms
            </td>
            <td style={{ ...td, ...r }}>
              <StateBadge row={row} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

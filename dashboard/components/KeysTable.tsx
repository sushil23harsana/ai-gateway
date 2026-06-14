import type { KeyStat } from "@/lib/types";
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

export default function KeysTable({ keys }: { keys: KeyStat[] }) {
  if (!keys.length) {
    return (
      <div style={{ color: "var(--text-tertiary)", fontSize: 13, padding: "28px 22px" }}>
        no virtual keys yet — mint one via POST /admin/keys
      </div>
    );
  }

  return (
    <table style={{ width: "100%", borderCollapse: "collapse" }}>
      <thead>
        <tr>
          <th style={th}>Key</th>
          <th style={{ ...th, ...r }}>RPM</th>
          <th style={{ ...th, width: "24%" }}>Budget (month)</th>
          <th style={{ ...th, ...r }}>Month</th>
          <th style={{ ...th, ...r }}>Total</th>
          <th style={{ ...th, ...r }}>Reqs</th>
          <th style={{ ...th, ...r }}>Status</th>
        </tr>
      </thead>
      <tbody>
        {keys.map((k) => {
          const ratio = k.monthly_budget_usd ? k.month_cost_usd / k.monthly_budget_usd : 0;
          const tone = ratio >= 1 ? "negative" : ratio >= 0.8 ? "warning" : "positive";
          return (
            <tr key={k.id}>
              <td style={{ ...td, fontWeight: 600 }}>{k.name}</td>
              <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
                {k.rate_limit_rpm}
              </td>
              <td style={td}>
                {k.monthly_budget_usd ? (
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <div style={{ flex: 1, minWidth: 90 }}>
                      <ProgressBar value={ratio * 100} tone={tone} height={6} glow={false} />
                    </div>
                    <span className="mono" style={{ fontSize: 12, color: "var(--text-secondary)" }}>
                      {usd(k.monthly_budget_usd)}
                    </span>
                  </div>
                ) : (
                  <span style={{ color: "var(--text-tertiary)" }}>— no cap</span>
                )}
              </td>
              <td className="mono" style={{ ...td, ...r }}>
                {usd(k.month_cost_usd)}
              </td>
              <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
                {usd(k.total_cost_usd)}
              </td>
              <td className="mono" style={{ ...td, ...r, color: "var(--text-secondary)" }}>
                {num(k.requests)}
              </td>
              <td style={{ ...td, ...r }}>
                <Badge tone={k.disabled ? "negative" : "positive"} dot>
                  {k.disabled ? "disabled" : "active"}
                </Badge>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

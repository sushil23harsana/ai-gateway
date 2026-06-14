import type { KeyStat } from "@/lib/types";
import { num, usd } from "@/lib/format";

export default function KeysTable({ keys }: { keys: KeyStat[] }) {
  if (!keys.length) {
    return <div className="empty">no virtual keys yet — mint one via POST /admin/keys</div>;
  }

  return (
    <table className="keys">
      <thead>
        <tr>
          <th>Key</th>
          <th className="r">RPM</th>
          <th>Budget (month)</th>
          <th className="r">Month</th>
          <th className="r">Total</th>
          <th className="r">Reqs</th>
          <th className="r">Status</th>
        </tr>
      </thead>
      <tbody>
        {keys.map((k) => {
          const ratio = k.monthly_budget_usd ? k.month_cost_usd / k.monthly_budget_usd : 0;
          const cls = ratio >= 1 ? "over" : ratio >= 0.8 ? "warn" : "";
          return (
            <tr key={k.id}>
              <td className="name">{k.name}</td>
              <td className="r">{k.rate_limit_rpm}</td>
              <td>
                {k.monthly_budget_usd ? (
                  <div className="budget">
                    <div className="budget-bar">
                      <div
                        className={`budget-fill ${cls}`}
                        style={{ width: `${Math.min(100, ratio * 100)}%` }}
                      />
                    </div>
                    <span>{usd(k.monthly_budget_usd)}</span>
                  </div>
                ) : (
                  <span style={{ color: "var(--faint)" }}>— no cap</span>
                )}
              </td>
              <td className="r">{usd(k.month_cost_usd)}</td>
              <td className="r">{usd(k.total_cost_usd)}</td>
              <td className="r">{num(k.requests)}</td>
              <td className="r">
                <span className={`pill ${k.disabled ? "off" : ""}`}>
                  {k.disabled ? "disabled" : "active"}
                </span>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

"use client";

import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import type { ProviderStat } from "@/lib/types";

const COLORS = ["#8a5cf6", "#5b6cff", "#3fd8e0", "#00e676", "#ffb23e"];

function Tip({ active, payload }: any) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div
      style={{
        background: "var(--surface-3)",
        border: "1px solid var(--border-default)",
        borderRadius: 10,
        padding: "9px 11px",
        fontSize: 12,
        fontFamily: "var(--font-mono)",
        color: "var(--text-primary)",
      }}
    >
      <div>{p.name}</div>
      <div>
        <span style={{ color: "var(--text-secondary)" }}>reqs </span>
        {p.value}
      </div>
      <div>
        <span style={{ color: "var(--text-secondary)" }}>cost </span>${Number(p.cost).toFixed(6)}
      </div>
    </div>
  );
}

export default function ByProviderChart({ providers }: { providers: ProviderStat[] }) {
  if (!providers.length) {
    return <div style={{ color: "var(--text-tertiary)", fontSize: 13, padding: "40px 0", textAlign: "center" }}>no requests yet</div>;
  }
  const data = providers.map((p) => ({ name: p.provider, value: p.requests, cost: p.cost_usd }));

  return (
    <>
      <ResponsiveContainer width="100%" height={180}>
        <PieChart>
          <Pie
            data={data}
            dataKey="value"
            nameKey="name"
            innerRadius={52}
            outerRadius={78}
            paddingAngle={data.length > 1 ? 2 : 0}
            stroke="none"
          >
            {data.map((_, i) => (
              <Cell key={i} fill={COLORS[i % COLORS.length]} />
            ))}
          </Pie>
          <Tooltip content={<Tip />} />
        </PieChart>
      </ResponsiveContainer>
      <div style={{ display: "flex", gap: 16, flexWrap: "wrap", marginTop: 8 }}>
        {data.map((d, i) => (
          <span key={d.name} style={{ display: "inline-flex", alignItems: "center", gap: 7, fontSize: 12, color: "var(--text-secondary)" }}>
            <i style={{ width: 9, height: 9, borderRadius: 3, background: COLORS[i % COLORS.length], display: "inline-block" }} />
            <span className="mono" style={{ color: "var(--text-primary)" }}>
              {d.name}
            </span>{" "}
            · {d.value}
          </span>
        ))}
      </div>
    </>
  );
}

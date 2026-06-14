"use client";

import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import type { ProviderStat } from "@/lib/types";

const COLORS = ["#c8f23e", "#5ad1e0", "#f5b14c", "#ff6a6a", "#58e3a0"];

function Tip({ active, payload }: any) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="chart-tip">
      <div>{p.name}</div>
      <div>
        <span className="k">reqs </span>
        {p.value}
      </div>
      <div>
        <span className="k">cost </span>${Number(p.cost).toFixed(6)}
      </div>
    </div>
  );
}

export default function ByProviderChart({ providers }: { providers: ProviderStat[] }) {
  if (!providers.length) return <div className="empty">no requests yet</div>;

  const data = providers.map((p) => ({ name: p.provider, value: p.requests, cost: p.cost_usd }));

  return (
    <>
      <ResponsiveContainer width="100%" height={188}>
        <PieChart>
          <Pie
            data={data}
            dataKey="value"
            nameKey="name"
            innerRadius={54}
            outerRadius={80}
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
      <div className="legend">
        {data.map((d, i) => (
          <span key={d.name}>
            <i className="swatch" style={{ background: COLORS[i % COLORS.length] }} />
            {d.name} · {d.value}
          </span>
        ))}
      </div>
    </>
  );
}

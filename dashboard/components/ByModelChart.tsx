"use client";

import { Bar, BarChart, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { ModelStat } from "@/lib/types";

// Trim the dated snapshot suffix for a readable axis label.
function shorten(model: string): string {
  return model.replace(/-\d{8}$/, "").replace(/-20\d\d-\d\d-\d\d$/, "");
}

function Tip({ active, payload }: any) {
  if (!active || !payload?.length) return null;
  const m = payload[0].payload as ModelStat;
  return (
    <div className="chart-tip">
      <div>{m.model}</div>
      <div>
        <span className="k">reqs </span>
        {m.requests}
      </div>
      <div>
        <span className="k">tokens </span>
        {m.tokens_in} / {m.tokens_out}
      </div>
      <div>
        <span className="k">cost </span>${Number(m.cost_usd).toFixed(6)}
      </div>
    </div>
  );
}

export default function ByModelChart({ models }: { models: ModelStat[] }) {
  if (!models.length) return <div className="empty">no model usage yet</div>;

  const data = models.slice(0, 8).map((m) => ({ ...m, label: shorten(m.model) }));
  const height = Math.max(150, data.length * 46);

  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart layout="vertical" data={data} margin={{ top: 4, right: 18, left: 8, bottom: 0 }}>
        <XAxis
          type="number"
          tickLine={false}
          axisLine={false}
          tickFormatter={(v) => "$" + Number(v).toFixed(4)}
        />
        <YAxis type="category" dataKey="label" width={158} tickLine={false} axisLine={false} />
        <Tooltip content={<Tip />} cursor={{ fill: "rgba(200,242,62,0.05)" }} />
        <Bar dataKey="cost_usd" radius={[0, 3, 3, 0]} barSize={16}>
          {data.map((_, i) => (
            <Cell key={i} fill="#c8f23e" />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  );
}

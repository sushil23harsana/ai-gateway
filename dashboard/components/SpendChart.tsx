"use client";

import {
  Area,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { TimeBucket } from "@/lib/types";

function label(iso: string, range: string): string {
  const d = new Date(iso);
  if (range === "24h") return d.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", hour12: false });
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

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
      <div style={{ color: "var(--text-secondary)" }}>{p.t}</div>
      <div>
        <span style={{ color: "var(--violet-300)" }}>reqs </span>
        {p.requests}
      </div>
      <div>
        <span style={{ color: "var(--cyan-400)" }}>cost </span>${Number(p.cost).toFixed(6)}
      </div>
    </div>
  );
}

export default function SpendChart({ buckets, range }: { buckets: TimeBucket[]; range: string }) {
  if (!buckets.length) {
    return <div style={{ color: "var(--text-tertiary)", fontSize: 13, padding: "40px 0", textAlign: "center" }}>no traffic in this window</div>;
  }

  const data = buckets.map((b) => ({
    t: label(b.timestamp, range),
    requests: b.requests,
    cost: Number(b.cost_usd.toFixed(6)),
  }));

  return (
    <ResponsiveContainer width="100%" height={230}>
      <ComposedChart data={data} margin={{ top: 8, right: 8, left: -14, bottom: 0 }}>
        <defs>
          <linearGradient id="reqFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#8a5cf6" stopOpacity={0.4} />
            <stop offset="100%" stopColor="#8a5cf6" stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <CartesianGrid stroke="rgba(255,255,255,0.05)" vertical={false} />
        <XAxis dataKey="t" tickLine={false} axisLine={{ stroke: "rgba(255,255,255,0.08)" }} minTickGap={26} />
        <YAxis yAxisId="l" tickLine={false} axisLine={false} width={34} allowDecimals={false} />
        <YAxis yAxisId="r" orientation="right" hide />
        <Tooltip content={<Tip />} cursor={{ stroke: "rgba(255,255,255,0.12)" }} />
        <Area
          yAxisId="l"
          type="monotone"
          dataKey="requests"
          stroke="#8a5cf6"
          strokeWidth={2}
          fill="url(#reqFill)"
          activeDot={{ r: 3, fill: "#9b7dff" }}
        />
        <Line yAxisId="r" type="monotone" dataKey="cost" stroke="#3fd8e0" strokeWidth={1.5} dot={false} />
      </ComposedChart>
    </ResponsiveContainer>
  );
}

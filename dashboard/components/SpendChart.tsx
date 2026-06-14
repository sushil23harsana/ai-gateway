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
    <div className="chart-tip">
      <div>{p.t}</div>
      <div>
        <span className="k">reqs </span>
        {p.requests}
      </div>
      <div>
        <span className="k">cost </span>${Number(p.cost).toFixed(6)}
      </div>
    </div>
  );
}

export default function SpendChart({ buckets, range }: { buckets: TimeBucket[]; range: string }) {
  if (!buckets.length) return <div className="empty">no traffic in this window</div>;

  const data = buckets.map((b) => ({
    t: label(b.timestamp, range),
    requests: b.requests,
    cost: Number(b.cost_usd.toFixed(6)),
  }));

  return (
    <>
      <ResponsiveContainer width="100%" height={230}>
        <ComposedChart data={data} margin={{ top: 8, right: 8, left: -14, bottom: 0 }}>
          <defs>
            <linearGradient id="reqFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#c8f23e" stopOpacity={0.32} />
              <stop offset="100%" stopColor="#c8f23e" stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="#1a1d25" vertical={false} />
          <XAxis dataKey="t" tickLine={false} axisLine={{ stroke: "#21252e" }} minTickGap={26} />
          <YAxis yAxisId="l" tickLine={false} axisLine={false} width={34} allowDecimals={false} />
          <YAxis yAxisId="r" orientation="right" hide />
          <Tooltip content={<Tip />} cursor={{ stroke: "#2a2f3a" }} />
          <Area
            yAxisId="l"
            type="monotone"
            dataKey="requests"
            stroke="#c8f23e"
            strokeWidth={2}
            fill="url(#reqFill)"
            activeDot={{ r: 3, fill: "#c8f23e" }}
          />
          <Line yAxisId="r" type="monotone" dataKey="cost" stroke="#f5b14c" strokeWidth={1.5} dot={false} />
        </ComposedChart>
      </ResponsiveContainer>
      <div className="legend">
        <span>
          <i className="swatch" style={{ background: "#c8f23e" }} />
          requests
        </span>
        <span>
          <i className="swatch" style={{ background: "#f5b14c" }} />
          spend (usd)
        </span>
      </div>
    </>
  );
}

"use client";

import { useEffect, useState } from "react";
import { Area, AreaChart, ResponsiveContainer } from "recharts";
import type { Live } from "@/lib/types";

const EMPTY: Live = { current_per_minute: 0, recent: [] };

export default function LiveTile() {
  const [live, setLive] = useState<Live>(EMPTY);

  useEffect(() => {
    let on = true;
    const tick = async () => {
      try {
        const r = await fetch("/api/live", { cache: "no-store" });
        if (on && r.ok) setLive(await r.json());
      } catch {
        /* keep last */
      }
    };
    tick();
    const id = setInterval(tick, 5000);
    return () => {
      on = false;
      clearInterval(id);
    };
  }, []);

  const data = live.recent.map((m) => ({ c: m.count }));

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
        <span className="mono" style={{ fontSize: 48, fontWeight: 700, lineHeight: 1, letterSpacing: "-0.02em", color: "var(--violet-200)" }}>
          {live.current_per_minute}
        </span>
        <span style={{ fontSize: 13, color: "var(--text-secondary)" }}>req / min</span>
      </div>
      <div style={{ flex: 1, minHeight: 0, marginTop: 12 }}>
        <ResponsiveContainer width="100%" height={90}>
          <AreaChart data={data} margin={{ top: 14, right: 0, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="liveFill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#8a5cf6" stopOpacity={0.45} />
                <stop offset="100%" stopColor="#8a5cf6" stopOpacity={0} />
              </linearGradient>
            </defs>
            <Area type="monotone" dataKey="c" stroke="#9b7dff" strokeWidth={1.5} fill="url(#liveFill)" isAnimationActive={false} />
          </AreaChart>
        </ResponsiveContainer>
      </div>
      <span style={{ fontSize: 11, color: "var(--text-tertiary)" }}>last 15 min · polling 5s</span>
    </div>
  );
}

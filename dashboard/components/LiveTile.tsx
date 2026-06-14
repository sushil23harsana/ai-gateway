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
        /* keep last value */
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
    <div>
      <div className="live-big">
        {live.current_per_minute}
        <span className="live-unit">req / min</span>
      </div>
      <ResponsiveContainer width="100%" height={92}>
        <AreaChart data={data} margin={{ top: 16, right: 0, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="liveFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#c8f23e" stopOpacity={0.4} />
              <stop offset="100%" stopColor="#c8f23e" stopOpacity={0} />
            </linearGradient>
          </defs>
          <Area
            type="monotone"
            dataKey="c"
            stroke="#c8f23e"
            strokeWidth={1.5}
            fill="url(#liveFill)"
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
      <div className="panel-note">last 15 min · polling 5s</div>
    </div>
  );
}

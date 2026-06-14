"use client";

import { useEffect, useState } from "react";
import { Activity } from "lucide-react";

// Compact live requests/min indicator in the topbar (polls /api/live).
export default function LiveChip() {
  const [rpm, setRpm] = useState(0);

  useEffect(() => {
    let on = true;
    const tick = async () => {
      try {
        const r = await fetch("/api/live", { cache: "no-store" });
        if (on && r.ok) {
          const d = await r.json();
          setRpm(d.current_per_minute ?? 0);
        }
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

  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 7,
        padding: "5px 11px",
        borderRadius: "var(--radius-pill)",
        background: "var(--fill-violet)",
        boxShadow: "inset 0 0 0 1px rgba(138,92,246,0.40)",
        color: "var(--violet-200)",
        fontSize: 12,
      }}
    >
      <Activity size={13} />
      <span className="mono" style={{ fontWeight: 600, color: "#fff" }}>
        {rpm}
      </span>
      <span style={{ color: "var(--violet-200)" }}>req/min</span>
    </span>
  );
}

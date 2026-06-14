"use client";

import { usePathname, useRouter } from "next/navigation";

const ITEMS = [
  { value: "24h", label: "24H" },
  { value: "7d", label: "7D" },
  { value: "30d", label: "30D" },
];

// Segmented range toggle. Updates ?range= so the server re-fetches the timeseries.
export default function RangePills({ active }: { active: string }) {
  const router = useRouter();
  const pathname = usePathname();

  return (
    <div
      role="tablist"
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 4,
        padding: 4,
        background: "var(--surface-inset)",
        borderRadius: "var(--radius-pill)",
        boxShadow: "var(--inner-border)",
      }}
    >
      {ITEMS.map((it) => {
        const on = it.value === active;
        return (
          <button
            key={it.value}
            type="button"
            role="tab"
            aria-selected={on}
            onClick={() => router.push(`${pathname}?range=${it.value}`)}
            style={{
              height: 30,
              padding: "0 14px",
              fontFamily: "var(--font-sans)",
              fontSize: 12,
              fontWeight: 600,
              border: "none",
              borderRadius: "var(--radius-pill)",
              cursor: "pointer",
              color: on ? "var(--text-primary)" : "var(--text-secondary)",
              background: on ? "var(--surface-2)" : "transparent",
              boxShadow: on ? "inset 0 0 0 1px rgba(255,255,255,0.09), var(--glow-violet-sm)" : "none",
              transition: "var(--transition-base)",
            }}
          >
            {it.label}
          </button>
        );
      })}
    </div>
  );
}

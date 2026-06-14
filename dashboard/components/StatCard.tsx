import type { ReactNode } from "react";
import Card from "./Card";

// Hero metric tile: eyebrow label, big tabular number, optional icon + footnote.
export default function StatCard({
  label,
  value,
  icon,
  footnote,
  accentValue = false,
  mesh = false,
  delay = 0,
}: {
  label: string;
  value: string;
  icon?: ReactNode;
  footnote?: string;
  accentValue?: boolean;
  mesh?: boolean;
  delay?: number;
}) {
  return (
    <Card mesh={mesh} padding="20px" style={{ minWidth: 0, animationDelay: `${delay}ms` }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 16, height: "100%" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
          <span className="eyebrow">{label}</span>
          {icon && (
            <span
              style={{
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "center",
                width: 30,
                height: 30,
                borderRadius: "var(--radius-sm)",
                background: "var(--fill-violet)",
                color: "var(--violet-300)",
                boxShadow: "var(--inner-border)",
              }}
            >
              {icon}
            </span>
          )}
        </div>

        <span
          className="mono"
          style={{
            fontSize: 30,
            fontWeight: 700,
            letterSpacing: "-0.02em",
            lineHeight: 1,
            whiteSpace: "nowrap",
            color: accentValue ? "var(--violet-200)" : "var(--text-primary)",
          }}
        >
          {value}
        </span>

        {footnote && (
          <span style={{ marginTop: "auto", fontSize: 12, color: "var(--text-tertiary)" }}>{footnote}</span>
        )}
      </div>
    </Card>
  );
}

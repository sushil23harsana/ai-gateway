import type { CSSProperties, ReactNode } from "react";

// Elevated surface; a 1px inner border lifts it off the dark canvas. `mesh`
// paints the diffused violet mesh used by feature/hero cards.
export default function Card({
  children,
  mesh = false,
  padding = "var(--space-6, 24px)",
  className = "",
  style,
}: {
  children: ReactNode;
  mesh?: boolean;
  padding?: string;
  className?: string;
  style?: CSSProperties;
}) {
  return (
    <div
      className={`reveal ${className}`}
      style={{
        position: "relative",
        borderRadius: "var(--radius-lg)",
        padding,
        background: mesh ? "var(--gradient-mesh)" : "var(--surface-card)",
        boxShadow: "var(--elev-card)",
        overflow: "hidden",
        ...style,
      }}
    >
      <div style={{ position: "relative", zIndex: 1, height: "100%" }}>{children}</div>
    </div>
  );
}

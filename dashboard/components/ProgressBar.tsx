type Tone = "violet" | "positive" | "negative" | "warning";

const FILLS: Record<Tone, string> = {
  violet: "var(--gradient-violet)",
  positive: "var(--gradient-positive)",
  negative: "linear-gradient(135deg, #ff5c7a 0%, #f43f66 100%)",
  warning: "linear-gradient(135deg, #ffb23e 0%, #ff8a3e 100%)",
};
const GLOWS: Record<Tone, string> = {
  violet: "0 0 12px rgba(138,92,246,0.5)",
  positive: "0 0 12px rgba(0,230,118,0.5)",
  negative: "0 0 12px rgba(255,92,122,0.5)",
  warning: "0 0 12px rgba(255,178,62,0.5)",
};

export default function ProgressBar({
  value,
  tone = "violet",
  height = 8,
  label,
  showValue = false,
  glow = true,
}: {
  value: number;
  tone?: Tone;
  height?: number;
  label?: string;
  showValue?: boolean;
  glow?: boolean;
}) {
  const pct = Math.max(0, Math.min(100, value));
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      {(label || showValue) && (
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
          {label && <span style={{ fontSize: 12, color: "var(--text-secondary)" }}>{label}</span>}
          {showValue && (
            <span className="mono" style={{ fontSize: 12, color: "var(--text-primary)" }}>
              {pct.toFixed(0)}%
            </span>
          )}
        </div>
      )}
      <div
        style={{
          position: "relative",
          width: "100%",
          height,
          background: "var(--surface-inset)",
          borderRadius: "var(--radius-pill)",
          boxShadow: "var(--inner-border)",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            position: "absolute",
            inset: 0,
            width: `${pct}%`,
            background: FILLS[tone],
            borderRadius: "var(--radius-pill)",
            boxShadow: glow ? GLOWS[tone] : "none",
          }}
        />
      </div>
    </div>
  );
}

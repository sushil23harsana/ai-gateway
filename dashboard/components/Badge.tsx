import type { ReactNode } from "react";

type Tone = "neutral" | "violet" | "positive" | "negative" | "warning" | "info";
type Variant = "soft" | "solid" | "outline";

const TONES: Record<Tone, { fg: string; bg: string; bd: string; solidBg: string; solidFg: string }> = {
  neutral: { fg: "var(--text-secondary)", bg: "var(--fill-soft)", bd: "rgba(255,255,255,0.12)", solidBg: "var(--surface-3)", solidFg: "var(--text-primary)" },
  violet: { fg: "var(--violet-300)", bg: "var(--fill-violet)", bd: "rgba(138,92,246,0.45)", solidBg: "var(--violet-400)", solidFg: "#fff" },
  positive: { fg: "var(--positive)", bg: "var(--positive-bg)", bd: "rgba(0,230,118,0.40)", solidBg: "var(--green-500)", solidFg: "#04210f" },
  negative: { fg: "var(--negative)", bg: "var(--negative-bg)", bd: "rgba(255,92,122,0.40)", solidBg: "var(--red-500)", solidFg: "#fff" },
  warning: { fg: "var(--warning)", bg: "var(--amber-soft)", bd: "rgba(255,178,62,0.40)", solidBg: "var(--amber-400)", solidFg: "#241600" },
  info: { fg: "var(--info)", bg: "var(--cyan-soft)", bd: "rgba(63,216,224,0.40)", solidBg: "var(--cyan-400)", solidFg: "#042224" },
};

export default function Badge({
  children,
  tone = "neutral",
  variant = "soft",
  dot = false,
}: {
  children: ReactNode;
  tone?: Tone;
  variant?: Variant;
  dot?: boolean;
}) {
  const t = TONES[tone];
  const variantStyle =
    variant === "solid"
      ? { background: t.solidBg, color: t.solidFg }
      : variant === "outline"
        ? { background: "transparent", color: t.fg, boxShadow: `inset 0 0 0 1px ${t.bd}` }
        : { background: t.bg, color: t.fg, boxShadow: `inset 0 0 0 1px ${t.bd}` };

  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        padding: dot ? "3px 10px 3px 8px" : "3px 10px",
        fontFamily: "var(--font-sans)",
        fontSize: 11,
        fontWeight: 600,
        letterSpacing: "0.02em",
        lineHeight: 1,
        borderRadius: "var(--radius-pill)",
        whiteSpace: "nowrap",
        ...variantStyle,
      }}
    >
      {dot && (
        <span style={{ width: 6, height: 6, borderRadius: "50%", background: t.fg, boxShadow: `0 0 8px ${t.fg}`, flexShrink: 0 }} />
      )}
      {children}
    </span>
  );
}

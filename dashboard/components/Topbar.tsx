import RangePills from "./RangePills";
import LiveChip from "./LiveChip";

export default function Topbar({
  title,
  subtitle,
  range = "24h",
  showRange = false,
  connected,
}: {
  title: string;
  subtitle: string;
  range?: string;
  showRange?: boolean;
  connected: boolean;
}) {
  return (
    <header
      style={{
        height: "var(--topbar-height)",
        flexShrink: 0,
        display: "flex",
        alignItems: "center",
        gap: 16,
        padding: "0 28px",
        background: "var(--glass-bg)",
        backdropFilter: "blur(16px)",
        WebkitBackdropFilter: "blur(16px)",
        boxShadow: "inset 0 -1px 0 var(--border-subtle)",
        position: "sticky",
        top: 0,
        zIndex: 100,
      }}
    >
      <div style={{ display: "flex", flexDirection: "column", lineHeight: 1.25, minWidth: 0 }}>
        <h1 style={{ margin: 0, fontSize: 20, fontWeight: 700, letterSpacing: "-0.01em", whiteSpace: "nowrap" }}>
          {title}
        </h1>
        <span
          style={{
            color: "var(--text-secondary)",
            fontSize: 12,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {subtitle}
        </span>
      </div>

      <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 14 }}>
        <LiveChip />
        {showRange ? <RangePills active={range} /> : null}
        <span
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: 7,
            fontSize: 11,
            fontWeight: 600,
            letterSpacing: "0.06em",
            textTransform: "uppercase",
            color: connected ? "var(--positive)" : "var(--negative)",
          }}
        >
          <span
            style={{
              width: 7,
              height: 7,
              borderRadius: "50%",
              background: "currentColor",
              animation: connected ? "pulse 2s infinite" : "none",
            }}
          />
          {connected ? "online" : "offline"}
        </span>
      </div>
    </header>
  );
}

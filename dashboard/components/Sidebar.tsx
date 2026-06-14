import {
  LayoutDashboard,
  Route,
  ScrollText,
  Database,
  KeyRound,
  Wallet,
  Settings,
} from "lucide-react";
import type { ReactNode } from "react";

const NAV = [
  { id: "overview", label: "Overview", icon: <LayoutDashboard size={18} />, active: true },
  { id: "routes", label: "Routes", icon: <Route size={18} /> },
  { id: "logs", label: "Live Logs", icon: <ScrollText size={18} /> },
  { id: "caches", label: "Caches", icon: <Database size={18} /> },
  { id: "keys", label: "API Keys", icon: <KeyRound size={18} /> },
];
const BOTTOM = [
  { id: "usage", label: "Usage & Billing", icon: <Wallet size={18} /> },
  { id: "settings", label: "Settings", icon: <Settings size={18} /> },
];

function NavItem({ icon, label, href, active }: { icon: ReactNode; label: string; href: string; active?: boolean }) {
  return (
    <a
      href={href}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        height: 42,
        padding: "0 12px",
        textDecoration: "none",
        borderRadius: "var(--radius-md)",
        fontSize: 14,
        fontWeight: active ? 600 : 500,
        color: active ? "var(--text-primary)" : "var(--text-secondary)",
        background: active ? "var(--fill-violet)" : "transparent",
        boxShadow: active ? "var(--glow-nav-active)" : "none",
      }}
    >
      <span style={{ display: "inline-flex", color: active ? "var(--violet-300)" : "inherit" }}>{icon}</span>
      <span style={{ flex: 1, whiteSpace: "nowrap" }}>{label}</span>
    </a>
  );
}

export default function Sidebar() {
  return (
    <aside
      className="nx-sidebar"
      style={{
        width: "var(--sidebar-width)",
        flexShrink: 0,
        height: "100%",
        background: "var(--surface-1)",
        boxShadow: "inset -1px 0 0 var(--border-subtle)",
        display: "flex",
        flexDirection: "column",
        padding: "18px 14px",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "0 8px 18px" }}>
        <span
          style={{
            width: 32,
            height: 32,
            borderRadius: 9,
            background: "var(--gradient-violet)",
            boxShadow: "var(--glow-violet-sm)",
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            color: "#fff",
            fontWeight: 800,
            fontSize: 16,
          }}
        >
          ◇
        </span>
        <span style={{ color: "var(--text-primary)", fontSize: 16, fontWeight: 700, letterSpacing: "-0.01em" }}>
          AI Gateway
        </span>
        <span
          style={{
            marginLeft: "auto",
            fontSize: 10,
            fontWeight: 600,
            letterSpacing: "0.08em",
            color: "var(--violet-200)",
            background: "var(--fill-violet)",
            padding: "3px 7px",
            borderRadius: "var(--radius-pill)",
            boxShadow: "var(--inner-border)",
          }}
        >
          SELF-HOSTED
        </span>
      </div>

      <nav style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        {NAV.map((n) => (
          <NavItem key={n.id} icon={n.icon} label={n.label} href="#" active={n.active} />
        ))}
      </nav>

      <div style={{ marginTop: "auto", display: "flex", flexDirection: "column", gap: 4 }}>
        {BOTTOM.map((n) => (
          <NavItem key={n.id} icon={n.icon} label={n.label} href="#" />
        ))}
      </div>
    </aside>
  );
}

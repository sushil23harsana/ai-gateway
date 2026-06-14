"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { LayoutDashboard, Route, ScrollText, Database, KeyRound } from "lucide-react";
import type { ReactNode } from "react";

const NAV = [
  { href: "/", label: "Overview", icon: <LayoutDashboard size={18} /> },
  { href: "/routes", label: "Routing", icon: <Route size={18} /> },
  { href: "/logs", label: "Live Logs", icon: <ScrollText size={18} /> },
  { href: "/caches", label: "Caches", icon: <Database size={18} /> },
  { href: "/keys", label: "API Keys", icon: <KeyRound size={18} /> },
];

function isActive(pathname: string, href: string): boolean {
  return href === "/" ? pathname === "/" : pathname.startsWith(href);
}

function NavLink({ href, icon, label, active }: { href: string; icon: ReactNode; label: string; active: boolean }) {
  return (
    <Link
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
        transition: "var(--transition-base)",
      }}
    >
      <span style={{ display: "inline-flex", color: active ? "var(--violet-300)" : "inherit" }}>{icon}</span>
      <span style={{ flex: 1, whiteSpace: "nowrap" }}>{label}</span>
    </Link>
  );
}

export default function Sidebar() {
  const pathname = usePathname() || "/";
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
          <NavLink key={n.href} href={n.href} icon={n.icon} label={n.label} active={isActive(pathname, n.href)} />
        ))}
      </nav>

      <div
        style={{
          marginTop: "auto",
          padding: "12px",
          borderRadius: "var(--radius-md)",
          background: "var(--surface-inset)",
          boxShadow: "var(--inner-border)",
          color: "var(--text-tertiary)",
          fontSize: 11,
          lineHeight: 1.5,
        }}
      >
        Self-hosted LLM gateway · OpenAI + Claude routing, caching &amp; cost analytics.
      </div>
    </aside>
  );
}

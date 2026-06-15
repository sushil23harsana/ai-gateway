import { NextRequest, NextResponse } from "next/server";

// Server-side guard for control-plane write routes. Even when the gateway only
// listens on localhost, the user's browser can still reach it — so a malicious
// site could try to forge requests (CSRF) or rebind DNS to 127.0.0.1. Two checks
// close that off:
//   1. Host allowlist — blocks DNS-rebinding (the attacker's hostname won't match).
//   2. Same-origin — if an Origin header is present it must match Host (blocks
//      classic cross-site requests). Absent Origin (curl, server-to-server) is
//      allowed: those aren't browser-driven cross-site attacks.
//
// For a reverse-proxy / non-localhost deployment, set DASHBOARD_ALLOWED_HOSTS to
// the public hostname(s), comma-separated.
const ALLOWED_HOSTS = (process.env.DASHBOARD_ALLOWED_HOSTS ?? "localhost,127.0.0.1,[::1],::1")
  .split(",")
  .map((s) => s.trim().toLowerCase())
  .filter(Boolean);

// hostname strips the port, handling IPv6 forms like "[::1]:3000".
function hostname(h: string | null): string | null {
  if (!h) return null;
  const lower = h.toLowerCase();
  if (lower.startsWith("[")) {
    const end = lower.indexOf("]");
    return end >= 0 ? lower.slice(0, end + 1) : lower;
  }
  const colon = lower.indexOf(":");
  return colon >= 0 ? lower.slice(0, colon) : lower;
}

// assertLocalRequest returns a 403 response if the request fails the guard, or
// null if it is allowed to proceed.
export function assertLocalRequest(req: NextRequest): NextResponse | null {
  const host = req.headers.get("host");
  const name = hostname(host);
  if (!name || !ALLOWED_HOSTS.includes(name)) {
    return NextResponse.json(
      { error: `host ${host ?? "(none)"} is not allowed; set DASHBOARD_ALLOWED_HOSTS to permit it` },
      { status: 403 },
    );
  }

  const origin = req.headers.get("origin");
  if (origin) {
    let originHost: string | null = null;
    try {
      originHost = new URL(origin).host.toLowerCase();
    } catch {
      originHost = null;
    }
    if (!originHost || originHost !== (host ?? "").toLowerCase()) {
      return NextResponse.json({ error: "cross-origin request blocked" }, { status: 403 });
    }
  }

  return null;
}

import { NextResponse } from "next/server";

// Proxies the gateway's live counter so the browser polls this route (token-free)
// instead of the gateway directly. ADMIN_TOKEN stays server-side.
const BASE = process.env.GATEWAY_URL ?? "http://localhost:8080";
const TOKEN = process.env.ADMIN_TOKEN ?? "change-me";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const res = await fetch(`${BASE}/admin/stats/live`, {
      headers: { Authorization: `Bearer ${TOKEN}` },
      cache: "no-store",
    });
    if (!res.ok) throw new Error(String(res.status));
    return NextResponse.json(await res.json());
  } catch {
    return NextResponse.json({ current_per_minute: 0, recent: [] });
  }
}

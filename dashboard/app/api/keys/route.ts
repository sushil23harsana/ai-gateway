import { NextRequest, NextResponse } from "next/server";
import { gatewayWrite } from "@/lib/api";
import { assertLocalRequest } from "@/lib/security";

export const dynamic = "force-dynamic";

// POST /api/keys — create a virtual key. Server-side: holds ADMIN_TOKEN, checks
// same-origin, and passes the gateway's response (including the one-time raw key)
// straight back to the browser.
export async function POST(req: NextRequest) {
  const blocked = assertLocalRequest(req);
  if (blocked) return blocked;

  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON body" }, { status: 400 });
  }

  const r = await gatewayWrite("POST", "/admin/keys", body);
  return new NextResponse(r.body, { status: r.status, headers: { "content-type": r.contentType } });
}

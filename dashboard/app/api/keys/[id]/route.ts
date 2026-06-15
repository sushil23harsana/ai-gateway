import { NextRequest, NextResponse } from "next/server";
import { gatewayWrite } from "@/lib/api";
import { assertLocalRequest } from "@/lib/security";

export const dynamic = "force-dynamic";

// PATCH /api/keys/:id — update a key's settings (disable, rename, rate limit,
// budget, cache toggle). DELETE /api/keys/:id — remove a key. Both are guarded
// and forward to the gateway control plane.
export async function PATCH(req: NextRequest, { params }: { params: { id: string } }) {
  const blocked = assertLocalRequest(req);
  if (blocked) return blocked;

  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON body" }, { status: 400 });
  }

  const r = await gatewayWrite("PATCH", `/admin/keys/${encodeURIComponent(params.id)}`, body);
  return new NextResponse(r.body, { status: r.status, headers: { "content-type": r.contentType } });
}

export async function DELETE(req: NextRequest, { params }: { params: { id: string } }) {
  const blocked = assertLocalRequest(req);
  if (blocked) return blocked;

  const r = await gatewayWrite("DELETE", `/admin/keys/${encodeURIComponent(params.id)}`);
  return new NextResponse(r.body || null, { status: r.status, headers: { "content-type": r.contentType } });
}

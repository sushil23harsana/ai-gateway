"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

// Periodically re-runs the server component fetch so the dashboard stays fresh
// without a full reload.
export default function AutoRefresh({ intervalMs = 20000 }: { intervalMs?: number }) {
  const router = useRouter();
  useEffect(() => {
    const id = setInterval(() => router.refresh(), intervalMs);
    return () => clearInterval(id);
  }, [router, intervalMs]);
  return null;
}

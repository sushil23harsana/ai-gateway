"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { num, usd } from "@/lib/format";
import Badge from "./Badge";
import ProgressBar from "./ProgressBar";

export type KeyRow = {
  id: string;
  name: string;
  rate_limit_rpm: number;
  monthly_budget_usd?: number;
  cache_enabled: boolean;
  disabled: boolean;
  created_at: string;
  month_cost_usd: number;
  total_cost_usd: number;
  requests: number;
};

type FormValues = {
  name: string;
  rate_limit_rpm: number;
  monthly_budget_usd?: number;
  cache_enabled: boolean;
};

// ---- shared styles (match the dashboard design system) ----
const th: React.CSSProperties = {
  textAlign: "left",
  padding: "10px 22px",
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: "0.06em",
  textTransform: "uppercase",
  color: "var(--text-secondary)",
};
const td: React.CSSProperties = {
  padding: "13px 22px",
  fontSize: 14,
  color: "var(--text-primary)",
  borderTop: "1px solid var(--border-subtle)",
};
const right: React.CSSProperties = { textAlign: "right" };

const btnPrimary: React.CSSProperties = {
  padding: "8px 15px",
  fontSize: 13,
  fontWeight: 600,
  color: "#fff",
  background: "var(--violet-400)",
  border: "1px solid var(--violet-500)",
  borderRadius: 8,
  cursor: "pointer",
};
const btnGhost: React.CSSProperties = {
  padding: "8px 15px",
  fontSize: 13,
  fontWeight: 600,
  color: "var(--text-secondary)",
  background: "transparent",
  border: "1px solid var(--border-strong)",
  borderRadius: 8,
  cursor: "pointer",
};
const btnSmall: React.CSSProperties = {
  padding: "5px 10px",
  fontSize: 12,
  fontWeight: 600,
  borderRadius: 7,
  cursor: "pointer",
  border: "1px solid var(--border-strong)",
  background: "var(--surface-2)",
  color: "var(--text-secondary)",
};
const fieldLabel: React.CSSProperties = {
  display: "block",
  fontSize: 12,
  color: "var(--text-secondary)",
  marginBottom: 6,
  fontWeight: 600,
};
const input: React.CSSProperties = {
  width: "100%",
  padding: "9px 11px",
  fontSize: 14,
  color: "var(--text-primary)",
  background: "var(--surface-inset)",
  border: "1px solid var(--border-default)",
  borderRadius: 8,
  outline: "none",
};
const overlay: React.CSSProperties = {
  position: "fixed",
  inset: 0,
  background: "rgba(5,5,10,0.62)",
  backdropFilter: "blur(2px)",
  display: "grid",
  placeItems: "center",
  zIndex: 50,
  padding: 20,
};
const modalCard: React.CSSProperties = {
  width: "100%",
  maxWidth: 440,
  background: "var(--surface-1)",
  border: "1px solid var(--border-strong)",
  borderRadius: "var(--radius-lg)",
  boxShadow: "var(--elev-card)",
  padding: 24,
};

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return (
    <div style={overlay} onClick={onClose}>
      <div style={modalCard} onClick={(e) => e.stopPropagation()}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 18 }}>
          <span style={{ fontSize: 16, fontWeight: 700 }}>{title}</span>
          <button onClick={onClose} aria-label="Close" style={{ ...btnGhost, padding: "4px 9px", fontSize: 15 }}>
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

function KeyFormModal({
  mode,
  initial,
  busy,
  onClose,
  onSubmit,
}: {
  mode: "create" | "edit";
  initial?: KeyRow;
  busy: boolean;
  onClose: () => void;
  onSubmit: (v: FormValues) => void;
}) {
  const [name, setName] = useState(initial?.name ?? "");
  const [rpm, setRpm] = useState(String(initial?.rate_limit_rpm ?? 60));
  const [budget, setBudget] = useState(initial?.monthly_budget_usd != null ? String(initial.monthly_budget_usd) : "");
  const [cache, setCache] = useState(initial?.cache_enabled ?? true);
  const [err, setErr] = useState<string | null>(null);

  function submit() {
    if (!name.trim()) return setErr("name is required");
    const rpmN = parseInt(rpm, 10);
    if (!Number.isFinite(rpmN) || rpmN <= 0) return setErr("RPM must be a positive number");
    let budgetN: number | undefined;
    if (budget.trim() !== "") {
      budgetN = parseFloat(budget);
      if (!Number.isFinite(budgetN) || budgetN < 0) return setErr("budget must be a number ≥ 0");
    }
    setErr(null);
    onSubmit({ name: name.trim(), rate_limit_rpm: rpmN, monthly_budget_usd: budgetN, cache_enabled: cache });
  }

  return (
    <Modal title={mode === "create" ? "New virtual key" : "Edit key"} onClose={onClose}>
      <div style={{ display: "flex", flexDirection: "column", gap: 15 }}>
        <div>
          <label style={fieldLabel}>Name</label>
          <input style={input} value={name} onChange={(e) => setName(e.target.value)} placeholder="team-frontend" autoFocus />
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
          <div>
            <label style={fieldLabel}>Rate limit (RPM)</label>
            <input style={input} value={rpm} onChange={(e) => setRpm(e.target.value)} inputMode="numeric" />
          </div>
          <div>
            <label style={fieldLabel}>Monthly budget ($)</label>
            <input
              style={input}
              value={budget}
              onChange={(e) => setBudget(e.target.value)}
              inputMode="decimal"
              placeholder={mode === "create" ? "none" : "unchanged"}
            />
          </div>
        </div>
        <label style={{ display: "flex", alignItems: "center", gap: 9, cursor: "pointer", fontSize: 13, color: "var(--text-primary)" }}>
          <input type="checkbox" checked={cache} onChange={(e) => setCache(e.target.checked)} />
          Response caching enabled
        </label>
        {mode === "edit" && (
          <div style={{ fontSize: 11, color: "var(--text-tertiary)" }} className="mono">
            blank budget = keep current (clearing a budget isn’t supported yet)
          </div>
        )}
        {err && <div style={{ fontSize: 12, color: "var(--negative)" }}>{err}</div>}
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 4 }}>
          <button style={btnGhost} onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button style={{ ...btnPrimary, opacity: busy ? 0.6 : 1 }} onClick={submit} disabled={busy}>
            {busy ? "Saving…" : mode === "create" ? "Create key" : "Save changes"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function RevealModal({ name, rawKey, onClose }: { name: string; rawKey: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(rawKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable; user can select manually */
    }
  }
  return (
    <Modal title={`Key created · ${name}`} onClose={onClose}>
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        <div style={{ fontSize: 13, color: "var(--text-secondary)", lineHeight: 1.5 }}>
          Copy this key now — it is shown <strong style={{ color: "var(--text-primary)" }}>once</strong> and cannot be
          retrieved again. Only its hash is stored.
        </div>
        <code
          className="mono"
          style={{
            display: "block",
            wordBreak: "break-all",
            fontSize: 13,
            padding: "12px 14px",
            background: "var(--surface-inset)",
            border: "1px solid var(--border-default)",
            borderRadius: 8,
            color: "var(--violet-200)",
          }}
        >
          {rawKey}
        </code>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
          <button style={btnGhost} onClick={copy}>
            {copied ? "Copied ✓" : "Copy"}
          </button>
          <button style={btnPrimary} onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </Modal>
  );
}

export default function KeysManager({ initialKeys, writesEnabled }: { initialKeys: KeyRow[]; writesEnabled: boolean }) {
  const router = useRouter();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [modal, setModal] = useState<{ mode: "create" } | { mode: "edit"; key: KeyRow } | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<KeyRow | null>(null);
  const [revealed, setRevealed] = useState<{ name: string; key: string } | null>(null);

  async function call(path: string, method: string, body?: unknown) {
    const res = await fetch(path, {
      method,
      headers: { "Content-Type": "application/json" },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    const text = await res.text();
    let data: { error?: string; key?: string; name?: string } = {};
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = { error: text };
      }
    }
    return { ok: res.ok, data };
  }

  async function submitCreate(form: FormValues) {
    setBusy("create");
    setError(null);
    const { ok, data } = await call("/api/keys", "POST", form);
    setBusy(null);
    if (!ok) return setError(data.error ?? "failed to create key");
    setModal(null);
    if (data.key) setRevealed({ name: data.name ?? form.name, key: data.key });
    router.refresh();
  }

  async function submitEdit(id: string, form: FormValues) {
    setBusy("edit");
    setError(null);
    const patch: Record<string, unknown> = {
      name: form.name,
      rate_limit_rpm: form.rate_limit_rpm,
      cache_enabled: form.cache_enabled,
    };
    if (form.monthly_budget_usd !== undefined) patch.monthly_budget_usd = form.monthly_budget_usd;
    const { ok, data } = await call(`/api/keys/${id}`, "PATCH", patch);
    setBusy(null);
    if (!ok) return setError(data.error ?? "failed to update key");
    setModal(null);
    router.refresh();
  }

  async function toggleDisabled(k: KeyRow) {
    setBusy(k.id);
    setError(null);
    const { ok, data } = await call(`/api/keys/${k.id}`, "PATCH", { disabled: !k.disabled });
    setBusy(null);
    if (!ok) setError(data.error ?? "failed to update key");
    else router.refresh();
  }

  async function doDelete(k: KeyRow) {
    setBusy(k.id);
    setError(null);
    const { ok, data } = await call(`/api/keys/${k.id}`, "DELETE");
    setBusy(null);
    setConfirmDelete(null);
    if (!ok) setError(data.error ?? "failed to delete key");
    else router.refresh();
  }

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "18px 22px" }}>
        <span style={{ fontSize: 16, fontWeight: 700 }}>Virtual keys</span>
        <button
          style={{ ...btnPrimary, opacity: writesEnabled ? 1 : 0.5, cursor: writesEnabled ? "pointer" : "not-allowed" }}
          onClick={() => writesEnabled && setModal({ mode: "create" })}
          disabled={!writesEnabled}
          title={writesEnabled ? "" : "Set a strong ADMIN_TOKEN to enable key management"}
        >
          + New key
        </button>
      </div>

      {!writesEnabled && (
        <div
          style={{
            margin: "0 22px 14px",
            padding: "11px 14px",
            fontSize: 12.5,
            color: "var(--warning)",
            background: "var(--amber-soft)",
            border: "1px solid rgba(255,178,62,0.35)",
            borderRadius: 8,
          }}
        >
          Key management is read-only. Set a strong <span className="mono">ADMIN_TOKEN</span> (not the default
          <span className="mono"> change-me</span>) to create, edit, or revoke keys.
        </div>
      )}

      {error && (
        <div
          style={{
            margin: "0 22px 14px",
            padding: "11px 14px",
            fontSize: 12.5,
            color: "var(--negative)",
            background: "var(--negative-bg)",
            border: "1px solid rgba(255,92,122,0.35)",
            borderRadius: 8,
          }}
        >
          {error}
        </div>
      )}

      {initialKeys.length === 0 ? (
        <div style={{ color: "var(--text-tertiary)", fontSize: 13, padding: "10px 22px 28px" }}>
          no virtual keys yet{writesEnabled ? " — click “New key” to mint one" : ""}.
        </div>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th style={th}>Key</th>
              <th style={{ ...th, ...right }}>RPM</th>
              <th style={{ ...th, width: "22%" }}>Budget (month)</th>
              <th style={{ ...th, ...right }}>Month</th>
              <th style={{ ...th, ...right }}>Reqs</th>
              <th style={th}>Cache</th>
              <th style={th}>Status</th>
              <th style={{ ...th, ...right }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {initialKeys.map((k) => {
              const ratio = k.monthly_budget_usd ? k.month_cost_usd / k.monthly_budget_usd : 0;
              const tone = ratio >= 1 ? "negative" : ratio >= 0.8 ? "warning" : "positive";
              const rowBusy = busy === k.id;
              return (
                <tr key={k.id} style={{ opacity: rowBusy ? 0.55 : 1 }}>
                  <td style={{ ...td, fontWeight: 600 }}>{k.name}</td>
                  <td className="mono" style={{ ...td, ...right, color: "var(--text-secondary)" }}>
                    {k.rate_limit_rpm}
                  </td>
                  <td style={td}>
                    {k.monthly_budget_usd ? (
                      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                        <div style={{ flex: 1, minWidth: 80 }}>
                          <ProgressBar value={ratio * 100} tone={tone} height={6} glow={false} />
                        </div>
                        <span className="mono" style={{ fontSize: 12, color: "var(--text-secondary)" }}>
                          {usd(k.monthly_budget_usd)}
                        </span>
                      </div>
                    ) : (
                      <span style={{ color: "var(--text-tertiary)" }}>— no cap</span>
                    )}
                  </td>
                  <td className="mono" style={{ ...td, ...right }}>
                    {usd(k.month_cost_usd)}
                  </td>
                  <td className="mono" style={{ ...td, ...right, color: "var(--text-secondary)" }}>
                    {num(k.requests)}
                  </td>
                  <td style={td}>
                    <Badge tone={k.cache_enabled ? "info" : "neutral"} variant="outline">
                      {k.cache_enabled ? "on" : "off"}
                    </Badge>
                  </td>
                  <td style={td}>
                    <Badge tone={k.disabled ? "negative" : "positive"} dot>
                      {k.disabled ? "disabled" : "active"}
                    </Badge>
                  </td>
                  <td style={{ ...td, ...right }}>
                    <div style={{ display: "inline-flex", gap: 7 }}>
                      <button
                        style={{ ...btnSmall, opacity: writesEnabled && !rowBusy ? 1 : 0.45, cursor: writesEnabled ? "pointer" : "not-allowed" }}
                        onClick={() => toggleDisabled(k)}
                        disabled={!writesEnabled || rowBusy}
                      >
                        {k.disabled ? "Enable" : "Disable"}
                      </button>
                      <button
                        style={{ ...btnSmall, opacity: writesEnabled && !rowBusy ? 1 : 0.45, cursor: writesEnabled ? "pointer" : "not-allowed" }}
                        onClick={() => setModal({ mode: "edit", key: k })}
                        disabled={!writesEnabled || rowBusy}
                      >
                        Edit
                      </button>
                      <button
                        style={{
                          ...btnSmall,
                          color: "var(--negative)",
                          borderColor: "rgba(255,92,122,0.4)",
                          opacity: writesEnabled && !rowBusy ? 1 : 0.45,
                          cursor: writesEnabled ? "pointer" : "not-allowed",
                        }}
                        onClick={() => setConfirmDelete(k)}
                        disabled={!writesEnabled || rowBusy}
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {modal?.mode === "create" && (
        <KeyFormModal mode="create" busy={busy === "create"} onClose={() => setModal(null)} onSubmit={submitCreate} />
      )}
      {modal?.mode === "edit" && (
        <KeyFormModal
          mode="edit"
          initial={modal.key}
          busy={busy === "edit"}
          onClose={() => setModal(null)}
          onSubmit={(v) => submitEdit(modal.key.id, v)}
        />
      )}
      {revealed && <RevealModal name={revealed.name} rawKey={revealed.key} onClose={() => setRevealed(null)} />}
      {confirmDelete && (
        <Modal title="Delete key?" onClose={() => setConfirmDelete(null)}>
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div style={{ fontSize: 13.5, color: "var(--text-secondary)", lineHeight: 1.5 }}>
              Permanently delete <strong style={{ color: "var(--text-primary)" }}>{confirmDelete.name}</strong>? Any app
              still using this key will start getting 401s. This cannot be undone.
            </div>
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
              <button style={btnGhost} onClick={() => setConfirmDelete(null)} disabled={busy === confirmDelete.id}>
                Cancel
              </button>
              <button
                style={{ ...btnPrimary, background: "var(--red-500)", borderColor: "var(--red-500)" }}
                onClick={() => doDelete(confirmDelete)}
                disabled={busy === confirmDelete.id}
              >
                {busy === confirmDelete.id ? "Deleting…" : "Delete key"}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

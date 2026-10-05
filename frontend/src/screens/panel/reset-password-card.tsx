import { useState } from "react";
import { Check, KeyRound, Minus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Callout } from "@/components/onboarding";
import { cn } from "@/lib/utils";
import { appliance } from "@/lib/appliance";

/**
 * Forgot-password recovery for this alpha: there's no email and no recovery
 * codes yet, so the only path back in is resetting the password directly —
 * which only someone with access to this computer (where the panel already
 * requires its own private link) can do. Collapsed by default to keep the
 * everyday Overview screen uncluttered.
 */
export function ResetPasswordCard({ disabled }: { disabled: boolean }) {
  const [open, setOpen] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  const passwordOk = password.length >= 12;
  const matchOk = confirm.length > 0 && confirm === password;
  const ready = username.trim().length > 0 && passwordOk && matchOk;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ready) return;
    setBusy(true); setError(""); setDone(false);
    try {
      await appliance("/reset-password", "POST", { username: username.trim(), password });
      setDone(true); setPassword(""); setConfirm("");
    } catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };

  if (!open) {
    return <section className="panel-card panel-card-pad" aria-label="Password recovery">
      <h2 className="panel-eyebrow">Forgot a password?</h2>
      <p className="panel-small">Reset any administrator's password directly from this computer. No email is needed.</p>
      <div className="panel-card-spacer" />
      <Button variant="soft" disabled={disabled} onClick={() => setOpen(true)}><KeyRound aria-hidden="true" />Reset a password</Button>
    </section>;
  }

  return <section className="panel-card panel-card-pad" aria-label="Password recovery">
    <h2 className="panel-eyebrow">Reset a password</h2>
    <Callout tone="danger" title="This takes effect immediately">
      Anyone who knows a username on this computer can set its password here. Only use this yourself, on your own computer.
    </Callout>
    <form className="on-stack" onSubmit={submit}>
      <div className="on-field">
        <label htmlFor="reset-username">Username</label>
        <Input id="reset-username" value={username} disabled={busy} autoComplete="username"
          placeholder="e.g. admin" onChange={e => setUsername(e.target.value)} />
      </div>
      <div className="on-field">
        <label htmlFor="reset-password">New password</label>
        <Input id="reset-password" type="password" value={password} disabled={busy} autoComplete="new-password"
          onChange={e => setPassword(e.target.value)} />
        <div className="on-chips">
          <span className={cn("on-chip", passwordOk && "on-ok")}>{passwordOk ? <Check aria-hidden="true" /> : <Minus aria-hidden="true" />}At least 12 characters</span>
        </div>
      </div>
      <div className="on-field">
        <label htmlFor="reset-confirm">Confirm new password</label>
        <Input id="reset-confirm" type="password" value={confirm} disabled={busy} autoComplete="new-password"
          aria-invalid={confirm.length > 0 && !matchOk} onChange={e => setConfirm(e.target.value)} />
        {confirm ? <p className={cn("on-hint", matchOk ? "on-success" : "on-error")} role="status">
          {matchOk ? "Both passwords match." : "These don't match. Type the same password again."}
        </p> : null}
      </div>
      {error ? <Callout tone="danger" title="Could not reset the password">{error}</Callout> : null}
      {done ? <Callout tone="ok" title="Password updated">That account can now sign in with the new password.</Callout> : null}
      <div className="panel-actions">
        <Button type="submit" variant="primary" disabled={busy || !ready}>{busy ? "Resetting…" : "Reset password"}</Button>
        <Button type="button" variant="ghost" disabled={busy} onClick={() => { setOpen(false); setError(""); setDone(false); }}>Close</Button>
      </div>
    </form>
  </section>;
}

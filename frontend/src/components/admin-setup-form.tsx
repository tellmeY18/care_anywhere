import { useState } from "react";
import { Check, Eye, EyeOff, Minus, UserPlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Callout } from "@/components/onboarding";
import { cn } from "@/lib/utils";

const USERNAME_PATTERN = /^[a-zA-Z0-9_-]{3,}$/;

/**
 * A single, self-explanatory form for the one thing this alpha needs: an
 * administrator account. Deliberately not the reused CARE Clinic admin step —
 * that component assumes a fixed "admin" username and ships recovery-code UI
 * this alpha doesn't have. Kept small and skimmable for a first-time,
 * non-technical reader.
 */
export function AdminSetupForm({ busy, error, onSubmit }: {
  busy: boolean;
  error: string;
  onSubmit: (username: string, password: string) => void;
}) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [show, setShow] = useState(false);
  const [touched, setTouched] = useState(false);

  const usernameOk = USERNAME_PATTERN.test(username);
  const passwordOk = password.length >= 12;
  const matchOk = confirm.length > 0 && confirm === password;
  const ready = usernameOk && passwordOk && matchOk;

  return (
    <form className="on-card on-pad on-password-card" onSubmit={e => { e.preventDefault(); setTouched(true); if (ready) onSubmit(username, password); }}>
      <div className="on-field">
        <label htmlFor="adminusername">Administrator username</label>
        <Input id="adminusername" value={username} disabled={busy} autoComplete="username"
          aria-describedby="username-hint" onChange={e => setUsername(e.target.value)} />
        <p id="username-hint" className={cn("on-hint", touched && !usernameOk && "on-error")} role="status">
          {touched && !usernameOk ? "Use 3 or more letters, numbers, dashes or underscores." : "You'll use this to sign in to CARE. You can add more staff accounts later."}
        </p>
      </div>
      <div className="on-field">
        <label htmlFor="adminpw">Password</label>
        <div className="on-input">
          <Input className="on-text-input" id="adminpw" type={show ? "text" : "password"} autoComplete="new-password"
            placeholder="Choose a password" value={password} disabled={busy}
            aria-describedby="password-hint" onChange={e => setPassword(e.target.value)} />
          <Button type="button" variant="ghost" aria-label={show ? "Hide password" : "Show password"} aria-pressed={show}
            disabled={busy} onClick={() => setShow(v => !v)}>{show ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}</Button>
        </div>
        <div className="on-chips">
          <span className={cn("on-chip", passwordOk && "on-ok")}>
            {passwordOk ? <Check aria-hidden="true" /> : <Minus aria-hidden="true" />}
            At least 12 characters
          </span>
        </div>
        <p id="password-hint" className="on-small">Save this somewhere safe — a password manager or written down. There is no recovery email in this alpha.</p>
      </div>
      <div className="on-field">
        <label htmlFor="adminpw-confirm">Confirm password</label>
        <Input className="on-text-input" id="adminpw-confirm" type={show ? "text" : "password"} autoComplete="new-password"
          placeholder="Type it again" value={confirm} disabled={busy}
          aria-invalid={confirm.length > 0 && !matchOk} aria-describedby="confirm-hint"
          onChange={e => setConfirm(e.target.value)} />
        <p id="confirm-hint" role="status" className={cn("on-hint", confirm && (matchOk ? "on-success" : "on-error"))}>
          {confirm ? (matchOk ? <><Check aria-hidden="true" />Both passwords match.</> : "These don't match. Type the same password again.") : null}
        </p>
      </div>
      {error ? <Callout tone="danger" title="Could not create the administrator">{error}</Callout> : null}
      <Button type="submit" variant="primary" className="on-save-primary" disabled={busy}>
        <UserPlus aria-hidden="true" />{busy ? "Creating…" : "Create administrator"}
      </Button>
    </form>
  );
}

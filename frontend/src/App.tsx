import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Callout } from "@/components/onboarding";
import { Spinner } from "@/components/spinner";
import { AdminSetupForm } from "@/components/admin-setup-form";
import { SetupLayout } from "@/screens/setup/setup-layout";
import { PanelScreen } from "@/screens/panel/panel-screen";
import { useCare } from "@/state/care-store";
import { appliance, type ApplianceStatus } from "@/lib/appliance";
import { useAppUpdate } from "@/hooks/use-app-update";

// Reuses CARE Clinic's setup chrome (progress rail, diagnostics layout); the
// administrator step is purpose-built for this appliance (see admin-setup-form.tsx).
export function App() {
  const care = useCare();
  const [state, setState] = useState<ApplianceStatus>({ healthy: false, configured: false, detail: "Checking on your clinic…", phase: "starting", platform: "", backupDir: "", stateDir: "" });
  const [working, setWorking] = useState(false);
  const [statusError, setStatusError] = useState("");
  const [setupError, setSetupError] = useState("");
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);
  const [diagnostics, setDiagnostics] = useState("");
  const [diagnosticsLoading, setDiagnosticsLoading] = useState(false);
  const update = useAppUpdate(true, false);

  useEffect(() => {
    let live = true;
    const poll = async () => { try { const s = await appliance("/status"); if (live) { setState(s); setStatusError(""); } } catch (e) { if (live) setStatusError(String(e)); } };
    void poll();
    const timer = setInterval(poll, 3000);
    return () => { live = false; clearInterval(timer); };
  }, []);
  useEffect(() => { if (state.configured && care.ready && care.flow !== "panel") care.openPanel(); }, [state.configured, care.ready, care.flow, care.openPanel]);
  useEffect(() => { setDiagnosticsOpen(false); setDiagnostics(""); }, [state.phase]);

  if (care.flow === "panel") return <PanelScreen />;

  const submit = async (username: string, password: string) => {
    setWorking(true); setSetupError("");
    try { await appliance("/setup", "POST", { username, password }); care.openPanel(); }
    catch (e) { setSetupError(String(e)); } finally { setWorking(false); }
  };
  const retry = async () => { setStatusError(""); try { await appliance("/start", "POST", {}); } catch (e) { setStatusError(String(e)); } };
  const loadDiagnostics = async () => {
    const opening = !diagnosticsOpen;
    setDiagnosticsOpen(opening);
    if (!opening || diagnostics) return;
    setDiagnosticsLoading(true);
    try { setDiagnostics(await appliance("/logs")); }
    catch (e) { setDiagnostics(`Could not load diagnostics: ${e}`); }
    finally { setDiagnosticsLoading(false); }
  };

  const isError = state.phase === "error";
  const title = state.healthy ? "Create your administrator account"
    : isError ? "CARE needs attention"
    : "Getting your clinic ready";
  const subtitle = state.healthy
    ? "This is your first sign-in for CARE. Add clinic details and staff after signing in."
    : state.detail;

  return <SetupLayout steps={["software", "admin"]} page={state.healthy ? "admin" : "software"} done={{ software: state.healthy }} working={working}
    title={title} subtitle={subtitle}
    note="CARE Anywhere alpha · local computer only" update={update}>
    {state.healthy ? (
      <AdminSetupForm busy={working} error={setupError} onSubmit={submit} />
    ) : isError ? (
      <Callout tone="danger" title="Something went wrong">
        {state.detail}
        <div className="on-actions">
          <Button onClick={retry}>Try again</Button>
          <Button variant="ghost" onClick={loadDiagnostics}>{diagnosticsOpen ? "Hide diagnostics" : "Show diagnostics"}</Button>
        </div>
      </Callout>
    ) : (
      <Callout title="Everything is included">
        CARE Anywhere runs entirely on this computer — no software downloads are needed once it's installed.
        This first check can take a minute or two.
        <div className="on-actions">
          {state.phase === "starting" ? <span className="on-row" role="status"><Spinner />Working…</span> : null}
          <Button variant="ghost" onClick={loadDiagnostics}>{diagnosticsOpen ? "Hide diagnostics" : "Show diagnostics"}</Button>
        </div>
      </Callout>
    )}
    {diagnosticsOpen ? (
      <Callout title="Diagnostics">
        <p className="on-small">
          This is a technical log of what CARE has been doing. It can help if you're asking someone for support —
          it does not contain patient data, but may contain file paths on this computer.
        </p>
        {diagnosticsLoading ? <span className="on-row" role="status"><Spinner />Loading…</span>
          : <pre className="on-mono" style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", maxHeight: 280, overflow: "auto" }}>{diagnostics || "No log output yet."}</pre>}
      </Callout>
    ) : null}
    {statusError && !isError ? <Callout tone="danger" title="Could not check on CARE">{statusError}</Callout> : null}
  </SetupLayout>;
}

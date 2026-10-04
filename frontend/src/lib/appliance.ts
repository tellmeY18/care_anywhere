// CARE Clinic bridge adapter: transport changes; the reused UI stays React/Wails-shaped.
const token = location.hash.slice(1) || sessionStorage.getItem("care-desktop-token");
if (token) {
  sessionStorage.setItem("care-desktop-token", token);
  history.replaceState(null, "", location.pathname);
}
export async function appliance(path: string, method = "GET", body?: unknown) {
  const response = await fetch(path, {
    method, headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await response.text();
  if (!response.ok) throw new Error(text);
  try { return JSON.parse(text); } catch { return text; }
}
const handlers = new Map<string, Set<(...data: any[]) => void>>();
function emit(event: string, ...args: any[]) { handlers.get(event)?.forEach(fn => fn(...args)); }
export function subscribe(event: string, fn: (...args: any[]) => void) {
  if (!handlers.has(event)) handlers.set(event, new Set());
  handlers.get(event)!.add(fn);
  return () => { handlers.get(event)?.delete(fn); };
}
const status = () => appliance("/status");
async function waitReady() {
  for (let i = 0; i < 300; i++) {
    const s = await status();
    if (s.healthy) return;
    if (s.phase === "error") throw new Error(s.detail);
    await new Promise(resolve => setTimeout(resolve, 2000));
  }
  throw new Error("CARE is taking longer than expected. Check the diagnostics.");
}
export const methods: Record<string, (...args: any[]) => Promise<any>> = {
  GetState: async () => { const s = await status(); return { role: "server", setup_done: s.configured, version: "0.1.0-alpha.1", platform: s.platform, mdns_name: "127.0.0.1:8484", restore_pending: false, plugin_recovery_pending: false, client_url: "" }; },
  UninstallRequested: async () => false,
  ClinicHealth: async () => { const s = await status(); return { active: s.healthy, code: s.healthy ? 200 : 503, detail: s.detail }; },
  ClinicStatus: async () => { const s = await status(); return s.phase === "stopped" ? "" : s.detail; },
  ClinicAction: async (action: string) => {
    if (!["start", "stop", "restart", "backup-now"].includes(action)) throw new Error("This action is not available in the appliance alpha.");
    void (async () => {
      try {
        if (action === "stop" || action === "restart") await appliance("/stop", "POST", {});
        if (action === "start" || action === "restart") {
          const s = await status();
          if (["stopped", "error"].includes(s.phase)) await appliance("/start", "POST", {});
          await waitReady();
        }
        if (action === "backup-now") {
          const s = await status();
          if (s.phase !== "stopped") throw new Error("Stop CARE before making this alpha's whole-disk backup.");
          await appliance("/backup", "POST", {});
        }
        emit("care-done", 0, action);
      } catch (e) { emit("care-error", "The operation did not finish", String(e)); emit("care-done", 1, action); }
    })();
  },
  ListBackups: () => appliance("/backups"),
  GetBackupDir: async () => (await status()).backupDir,
  GetBackupPolicy: async () => ({ interval_seconds: 0, retention_days: 0 }),
  StorageStatus: () => appliance("/storage"),
  RecheckStorage: () => appliance("/storage"),
  AutostartEnabled: async () => false,
  WasAutostartLaunched: async () => false,
  OpenURL: async (url: string) => {
    const target = url === "https://127.0.0.1:8484/" ? "http://127.0.0.1:8484/" : url;
    if (!/^https?:\/\//.test(target)) throw new Error("Unsupported URL");
    window.open(target, "_blank", "noopener,noreferrer");
  },
  OpenBackupDir: () => appliance("/backups-folder", "POST", {}),
  OpenLogFile: async () => { const text = await appliance("/logs"); const url = URL.createObjectURL(new Blob([text], {type:"text/plain"})); const a=document.createElement("a"); a.href=url; a.download="care-diagnostics.txt"; a.click(); setTimeout(()=>URL.revokeObjectURL(url),1000); },
  CheckAppUpdate: async () => ({ current: "0.1.0-alpha.1", version: "0.1.0-alpha.1", available: false }),
};

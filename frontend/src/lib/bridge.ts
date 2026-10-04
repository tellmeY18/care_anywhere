// CARE Clinic's UI contract, backed by the appliance HTTP adapter.
import { methods, subscribe } from "./appliance";
type CareBridge = Window["go"]["main"]["App"];

export const bridge = new Proxy({} as CareBridge, {
  get(_target, method) {
    if (typeof method !== "string") return undefined;
    return async (...args: unknown[]) => {
      const fn = methods[method];
      if (!fn) throw new Error(`"${method}" is not available in the appliance alpha.`);
      return fn(...args);
    };
  },
});

export type CareEvent =
  | "care-log" | "care-done" | "care-error" | "setup-done" | "setup-failed"
  | "uninstalled" | "care-update" | "care-check" | "care-storage"
  | "admin-recovery-codes-changed" | "app-update-progress"
  | "prereq-download-progress" | "client-connect-progress" | "quit-requested"
  | "confirmation-requested" | "confirmation-cancelled";

export function onCareEvent(event: CareEvent, handler: (...data: any[]) => void): () => void {
  return subscribe(event, handler);
}

// Frontend diagnostics belong to the browser console; VM diagnostics stay on the host.
export function logToHost(line: string): void { console.info(line); }

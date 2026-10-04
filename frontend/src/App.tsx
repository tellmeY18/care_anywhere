import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Callout } from "@/components/onboarding";
import { AdminStep } from "@/screens/setup/configuration-steps";
import { SetupLayout } from "@/screens/setup/setup-layout";
import { PanelScreen } from "@/screens/panel/panel-screen";
import { EMPTY_RECOVERY } from "@/screens/setup/setup-model";
import { EMPTY_SETUP_FORM } from "@/state/forms";
import { useCare } from "@/state/care-store";
import { appliance } from "@/lib/appliance";
import { useAppUpdate } from "@/hooks/use-app-update";

// Reuse CARE Clinic's setup layout and administrator step; VM preparation replaces installation.
export function App() {
  const care = useCare();
  const [form, setForm] = useState(EMPTY_SETUP_FORM);
  const [state, setState] = useState({healthy:false, detail:"Preparing your offline clinic…", phase:"starting"});
  const [working, setWorking] = useState(false);
  const [error, setError] = useState("");
  const update = useAppUpdate(true, false);
  useEffect(() => { let live=true; const poll=async()=>{try{const s=await appliance("/status");if(live)setState(s)}catch(e){if(live)setError(String(e))}}; void poll();const timer=setInterval(poll,3000);return()=>{live=false;clearInterval(timer)}; },[]);
  useEffect(()=>{ if ((state as any).configured && care.ready && care.flow!=="panel") care.openPanel(); },[state,care.ready,care.flow]);
  if (care.flow === "panel") return <PanelScreen />;
  const strong = form.adminPassword.length >= 12;
  const submit = async () => {
    if (!strong || form.adminPassword !== form.adminConfirm) return;
    setWorking(true);setError("");
    try { await appliance("/setup","POST",{username:"admin",password:form.adminPassword});care.openPanel(); }
    catch(e){setError(String(e))} finally{setWorking(false)}
  };
  return <SetupLayout steps={["software","admin"]} page={state.healthy?"admin":"software"} done={{software:state.healthy}} working={working}
    title={state.healthy?"Creating the admin password":"Preparing CARE"} subtitle={state.healthy?"Your first sign-in for CARE. Add clinic details and staff after signing in.":state.detail}
    note="CARE Anywhere alpha · local computer only" next={submit} nextDisabled={!state.healthy||!strong||form.adminPassword!==form.adminConfirm} update={update}>
    {state.healthy ? <AdminStep form={form} patch={values=>setForm(f=>({...f,...values}))} strength={{strong,message:"Use at least 12 characters."}} passwordError="" folderProblem="" recovery={EMPTY_RECOVERY} recoveryError="" busy={working} action="" onSave={()=>{}} onPrint={()=>{}} onReload={()=>{}} onBackups={()=>{}} onOpenFolder={()=>{}} />
      : <Callout title={state.phase==="error"?"CARE needs attention":"Everything is included"}>The appliance is checked before startup. No software downloads are needed.{state.phase==="error"?<Button onClick={()=>void appliance("/start","POST",{})}>Try again</Button>:null}</Callout>}
    {error?<Callout tone="danger" title="Could not continue">{error}</Callout>:null}
  </SetupLayout>;
}

import { Rail } from "@/components/rail";
import { Screen, ScreenBody } from "@/components/screen";
import { useCare } from "@/state/care-store";
import { OverviewTab } from "./overview-tab";
import { BackupsTab } from "./backups-tab";
import { StorageTab } from "./storage-tab";
import { PanelUpdateLockProvider } from "./panel-update-lock";
import { PanelNotice, PanelLogButton } from "./panel-ui";
import "./panel.css";

// CARE Clinic panel composition, limited to operations implemented by the VM adapter.
export function PanelScreen() {
  const care=useCare();
  return <PanelUpdateLockProvider active={false} isActive={()=>false}>
    <div className="care-panel"><Rail variant="panel"/><Screen className="care-panel-main">
      {care.operationError?<PanelNotice title={care.operationError.title}>{care.operationError.message}<PanelLogButton/></PanelNotice>:null}
      <ScreenBody className="panel-tab-body care-panel-content">
        <div hidden={care.tab!=="overview"}><OverviewTab onDiagnose={()=>{}}/></div>
        <div hidden={care.tab!=="backups"}><BackupsTab/></div>
        <div hidden={care.tab!=="storage"}><StorageTab/></div>
      </ScreenBody>
    </Screen></div>
  </PanelUpdateLockProvider>;
}

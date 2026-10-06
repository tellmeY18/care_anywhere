{ pkgs, frontend }:
pkgs.buildNpmPackage {
  pname = "care-frontend";
  version = "preview";
  src = frontend;
  postPatch = ''cp ${./frontend-lock.json} package-lock.json'';
  npmDepsHash = "sha256-/kOAfNQjvfi19UiGPwoyCCscIgZ6+o3ChPj/EMnEBco=";
  # Pinned at 22, not nixpkgs' current default: nodejs_24 breaks this
  # frontend's Vite/Rolldown build (a warning gets thrown as a fatal error;
  # the Nix sandbox only surfaces the stack trace, not the warning message).
  # Re-attempt the upgrade once that's root-caused.
  nodejs = pkgs.nodejs_22;
  npmFlags = [ "--ignore-scripts" "--legacy-peer-deps" ];
  REACT_CARE_API_URL = "";
  HUSKY = "0";
  installPhase = ''mkdir -p $out; cp -r build/. $out/'';
}

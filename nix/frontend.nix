{ pkgs, frontend }:
pkgs.buildNpmPackage {
  pname = "care-frontend";
  version = "preview";
  src = frontend;
  postPatch = ''cp ${./frontend-lock.json} package-lock.json'';
  npmDepsHash = "sha256-/kOAfNQjvfi19UiGPwoyCCscIgZ6+o3ChPj/EMnEBco=";
  nodejs = pkgs.nodejs_24;
  npmFlags = [ "--ignore-scripts" "--legacy-peer-deps" ];
  REACT_CARE_API_URL = "";
  HUSKY = "0";
  installPhase = ''mkdir -p $out; cp -r build/. $out/'';
}

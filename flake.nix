{
  description = "CARE Anywhere native clinic appliance";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    # Pinned separately and NOT following the main nixpkgs: upstream minio/minio
    # is abandoned and later nixpkgs revisions refuse to evaluate it as insecure
    # (CVE-2026-40344 and others — see nix/guest.nix). This is the last commit
    # before that marker, same RELEASE.2025-10-15T17-29-55Z build. This does not
    # fix the underlying vulnerabilities; migrate off MinIO before any
    # non-alpha release (see AGENTS.md).
    nixpkgs-minio.url = "github:NixOS/nixpkgs/e02e1158ced6773f143173828be86364b2a40379";
    microvm = {
      url = "github:microvm-nix/microvm.nix/a4251ae36520c59a31afc07ffc32aa4fd7f214d2";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    care = { url = "github:ohcnetwork/care/3fe704930d9b2d7b6ffdc212cbf6a6a72bfaede4"; flake = false; };
    frontend = { url = "github:ohcnetwork/care_fe/2546061ece7f2f8a10e6ff669ad3bceed2fb830f"; flake = false; };
  };
  outputs = { self, nixpkgs, nixpkgs-minio, microvm, care, frontend }:
    let
      systems = [ "aarch64-linux" "x86_64-linux" ];
      forSystems = nixpkgs.lib.genAttrs systems;
    in {
      nixosConfigurations = forSystems (system: nixpkgs.lib.nixosSystem {
        inherit system;
        specialArgs = { inherit self care frontend; minioPackage = nixpkgs-minio.legacyPackages.${system}.minio; };
        modules = [ microvm.nixosModules.microvm ./nix/guest.nix ];
      });
      packages = forSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          bundle = import ./nix/bundle.nix { inherit pkgs; guest = self.nixosConfigurations.${system}; };
          default = self.packages.${system}.bundle;
        });
    };
}

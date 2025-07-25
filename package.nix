{ buildGo124Module
, self
, lib
}:
buildGo124Module {
  pname = "ts-proxy";
  version = "${builtins.readFile ./version.txt}-${self.shortRev or self.dirtyShortRev or "rev"}";

  src = ./.;

  env.CGO_ENABLED = 0;

  # vendorHash = "sha256:${lib.fakeSha256}";
  vendorHash = "sha256-FPp4yA6UeCffGdZ5fUVLYAa9VWHGIEPxs7oSWuCZn/A=";

  postConfigure = ''
    # chmod -R +w vendor/gvisor.dev/gvisor #/pkg/refs/refs_template.go
    # rm vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go
    # substituteInPlace vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go \
    #   --replace refs_template refs
  '';

  meta.mainProgram = "ts-proxyd";
}

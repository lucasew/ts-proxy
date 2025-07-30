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
  vendorHash = "sha256-pcRu1vViqMRpH+c+2tCpQ3DAZWF8i3czUKHz9ne+DP0=";

  postConfigure = ''
    # chmod -R +w vendor/gvisor.dev/gvisor #/pkg/refs/refs_template.go
    # rm vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go
    # substituteInPlace vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go \
    #   --replace refs_template refs
  '';

  meta.mainProgram = "ts-proxyd";
}

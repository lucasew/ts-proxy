{ buildGo123Module
, self
}:
buildGo123Module {
  pname = "ts-proxy";
  version = "${builtins.readFile ./version.txt}-${self.shortRev or self.dirtyShortRev}";

  src = ./.;

  CGO_ENABLED = 0;

  # vendorHash = "sha256:${lib.fakeSha256}";
  vendorHash = "sha256-8fyyGXSzM5B0u71o+Np8/JvfP0dR9O48cNMt0YsnPR4=";

  postConfigure = ''
    # chmod -R +w vendor/gvisor.dev/gvisor #/pkg/refs/refs_template.go
    # rm vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go
    # substituteInPlace vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go \
    #   --replace refs_template refs
  '';

  meta.mainProgram = "ts-proxyd";
}

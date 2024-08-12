{ buildGo122Module
, lib
}:
buildGo122Module {
  name = "ts-proxy";

  src = ./.;

  CGO_ENABLED = 0;

  # vendorHash = "sha256:${lib.fakeSha256}";
  vendorHash = "sha256-0eDBLbBVnR/QYPw8IiUoPbr5CkN8Ck9AnwAuvI50UzA=";

  postConfigure = ''
    # chmod -R +w vendor/gvisor.dev/gvisor #/pkg/refs/refs_template.go
    # rm vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go
    # substituteInPlace vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go \
    #   --replace refs_template refs
  '';
}

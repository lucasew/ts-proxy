{
  lib,
  config,
  pkgs,
  ...
}:

let
  cfg = config.services.ts-proxy;

  yamlFormat = pkgs.formats.yaml { };

  # Build the YAML config from NixOS module options
  configContent = {
    state_dir = "/data";
    stop_on_fail = cfg.stopOnFail;

    tokens = lib.mapAttrs (
      name: tok: {
        auth_key = "\${${tok.authKeyVariable}}";
      }
    ) cfg.tokens;

    servers = lib.mapAttrs (
      name: host:
      {
        hostname = host.name;
        token = host.token;
        handlers = [
          (
            {
              type = if host.enableRaw then "tcp" else "http";
              upstream_address = host.address;
              upstream_network = if host.network != "" then host.network else "tcp";
              funnel = host.enableFunnel;
              tls = host.enableTLS;
            }
            // (lib.optionalAttrs (host.listen != 0) {
              listen = ":${toString host.listen}";
            })
          )
        ];
      }
    ) cfg.hosts;
  };

  configFile = yamlFormat.generate "ts-proxy.yaml" configContent;

  allProxies = lib.unique (lib.concatMap (host: host.proxies) (builtins.attrValues cfg.hosts));
in

{
  options = {
    services.ts-proxy = {
      network-domain = lib.mkOption {
        description = "Which ts.net domain this machine belongs";
        default = "stargazer-shark.ts.net";
      };

      environmentFile = lib.mkOption {
        description = "Path to environment file for ts-proxy credentials";
        default = "/run/secrets/ts-proxy";
      };

      image = lib.mkOption {
        description = "Which ts-proxy image to use";
        default = "ghcr.io/lucasew/ts-proxy:latest";
        type = lib.types.str;
      };

      user = lib.mkOption {
        description = "Service user";
        type = lib.types.str;
        default = "tsproxy";
      };
      group = lib.mkOption {
        description = "Service group";
        type = lib.types.str;
        default = "tsproxy";
      };

      dataDir = lib.mkOption {
        description = "Data dir";
        type = lib.types.str;
        default = "/var/lib/ts-proxy";
      };

      stopOnFail = lib.mkOption {
        description = "Stop all servers if any one fails";
        type = lib.types.bool;
        default = false;
      };

      serviceName = lib.mkOption {
        description = "Name of the systemd service for the combined ts-proxy instance";
        type = lib.types.str;
        default = "ts-proxy";
      };

      tokens = lib.mkOption {
        description = "Named authentication tokens. Each maps to an environment variable containing the auth key.";
        default = {
          default = { };
        };
        type = lib.types.attrsOf (
          lib.types.submodule {
            options = {
              authKeyVariable = lib.mkOption {
                description = "Environment variable name containing the Tailscale auth key";
                type = lib.types.str;
                default = "TS_AUTHKEY";
              };
            };
          }
        );
      };

      hosts = lib.mkOption {
        description = "Services to expose via ts-proxy";

        default = { };

        type = lib.types.attrsOf (
          lib.types.submodule (
            { name, ... }:
            {
              options = {
                enableFunnel = lib.mkEnableOption "enable funnel for this endpoint";
                enableTLS = lib.mkEnableOption "enable TLS for this endpoint";
                enableRaw = lib.mkEnableOption "treat this endpoint as a raw TCP socket";

                network = lib.mkOption {
                  description = "First parameter of net.Dial";
                  type = lib.types.str;
                  default = "";
                };

                address = lib.mkOption {
                  description = "Second parameter of net.Dial";
                  type = lib.types.str;
                };

                listen = lib.mkOption {
                  description = "Which port to listen in the vhost";
                  type = lib.types.port;
                  default = 0;
                };

                name = lib.mkOption {
                  description = "Service name";
                  type = lib.types.str;
                  default = name;
                };

                token = lib.mkOption {
                  description = "Which named token to use for authentication";
                  type = lib.types.str;
                  default = "default";
                };

                proxies = lib.mkOption {
                  description = "Which units this ts-proxy instance is proxying.";
                  type = lib.types.listOf lib.types.str;
                  default = [ ];
                };

                unitName = lib.mkOption {
                  description = "Systemd unit of the proxy (kept for compatibility)";
                  type = lib.types.str;
                  default = "ts-proxy-${name}";
                };
              };

            }
          )
        );
      };
    };
  };

  config = lib.mkIf (cfg.hosts != { }) {
    sops.secrets.ts-proxy = {
      sopsFile = ../../../../secrets/ts-proxy.env;
      owner = cfg.user;
      group = cfg.group;
      format = "dotenv";
    };

    users.users.${cfg.user} = {
      isSystemUser = true;
      inherit (cfg) group;
    };

    users.groups.${cfg.group} = { };

    systemd.tmpfiles.rules = [ "d ${cfg.dataDir} 0700 ${cfg.user} ${cfg.group} - -" ];

    systemd.slices.ts-proxys.sliceConfig = {
      CPUQuota = "10%";
      MemoryHigh = "256M";
      MemoryMax = "384M";
    };

    virtualisation.oci-containers.containers.${cfg.serviceName} = {
      inherit (cfg) image;
      pull = "always";
      serviceName = cfg.serviceName;
      extraOptions = [ "--network=host" ];
      environmentFiles = [ cfg.environmentFile ];
      volumes = [
        "${cfg.dataDir}:/data"
        "${configFile}:/etc/ts-proxy/config.yaml:ro"
      ];
      cmd = [
        "server"
        "--config"
        "/etc/ts-proxy/config.yaml"
      ];
    };

    systemd.services.${cfg.serviceName} = {
      description = "ts-proxy Tailscale reverse proxy";
      wantedBy = if allProxies == [ ] then [ "multi-user.target" ] else allProxies;

      after = allProxies;
      partOf = allProxies;
      wants = allProxies;

      restartIfChanged = true;

      serviceConfig = {
        Slice = "ts-proxy.slice";
        Restart = "always";
        RestartSec = "10s";
      };
    };
  };
}

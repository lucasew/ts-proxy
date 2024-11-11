# ts-proxy

Simple proxy program to allow exposing individual services to a Tailnet, and
even to the Internet using Tailscale Funnel.

Unfortunately, the Tailscale daemon only allows exposing services using the
current node domain, and you can't spawn (so far) nodes for services. With this
you can!

On first run for one service, you will have to authenticate the service using
your Tailscale account. The authentication can be either done passing an
authentication token through the `TS_AUTHKEY` environment or by reading the
startup logs until you find the authentication link. After authentication,
Tailscale will store the certificates and credentials to the location specified
by the `-s` flag so subsequent runs will not require reauthentication and up-to-date authorization tokens.

As the [main build of Tailscale](https://tailscale.com/s/serve-headers), you
can get information about the user accessing the service using the following
headers that get forwarded to the upstream service:
- `Tailscale-User-Login`
- `Tailscale-User-Name`
- `Tailscale-User-Profile-Pic`

And yeah, you can use Tailscale as a single sign on and have a public facing
version! It's as safe and stable as
[tclip](https://github.com/tailscale-dev/tclip) is because this proxy uses the
exact same primitives.

> [!WARNING] About header authentication security
> You can count on the headers sent by ts-proxy as long as you follow the following conditions:
> - Anything that changes the headers name representation such as Apache with PHP could be cheated
> by passing the header TAILSCALE_USER_LOGIN, for example.
>
> - If some users can access your actual service directly without passing the traffic through ts-proxy
they can change all the headers they want, including authentication ones.
>
> - If you don't use the header authentication for anything in a given service these issues will not be a problem for that service.


## Usage

```
Usage of ./ts-proxyd:
  -addr string
    	Port to listen (default ":443")
  -f	Enable tailscale funnel
  -h string
    	Where to forward the connection
  -n string
    	Hostname in tailscale devices list
  -s string
    	State directory
```

# zwiebelproxy

this is a small setup to access TOR services via a custom domain from the internet like the `tor2web` service. You can specify a domain, for example `onion.tld` and access a tor hidden service via `toraddress.onion.tld` without the need for a local tor setup. All you need to do is point a wildcard domain at this server.

Once the server receives an request it will check the domain and then forward it to the tor network and returns the reponse. This way you can setup some kind of a proxy into the tor network.

## DNS setup

Create an `*.onion.tld` CNAME record pointing to your server. Additionally you can also create a `onion.tld` CNAME pointing to the same server to see a nice page when calling `onion.tld` in the browser.

## local instructions

create a `.env` file with the required env variables or supply the parameters. View the `--help` for more information.

you can run `./start.sh` or use `docker compose up` to start the service.

## Letsencrypt / certbot

To use it with certbot and local certificates, you need to use a deploy hook as the private key is only readable by the root user by default and the docker container runs as a non priviledged user.

Example:

```
certbot certonly --dns-cloudflare --dns-cloudflare-credentials /root/.secrets/certbot/cloudflare.ini -d 'onion.tld' -d '*.onion.tld' --deploy-hook "cp -L /etc/letsencrypt/live/onion.tld/*.pem /root/zwiebelproxy/certs/; chmod 0644 /root/zwiebelproxy/certs/*.pem"
```

## Access restrictions

If you want to have a private tor proxy there are several access restrictions in place that can be configured.

### IP-ACL

By setting the `allowed-ips` option (or via the `ZWIEBEL_ALLOWED_IPS` env variable) you can specify multiple ips separated by comma that should be allowed to access this webserver. All other ips will be blocked.

### IP Range ACL

By setting the `allowed-ip-ranges` option (or via the `ZWIEBEL_ALLOWED_IPRANGES` env variable) you can specify multiple ip randes in CIDR notation separated by comma that should be allowed to access this webserver. All other ips will be blocked.

### Host ACL

By setting the `allowed-hosts` option (or via the `ZWIEBEL_ALLOWED_HOSTS` env variable) you can specify multiple dns names that should be allowed to access this server. Upon a request all configured `allowed-hosts` are resolved to the current ip adress and checked against the requesting ip. This way you can access the site from a non static ip if you have dyndns set up.

### Public

If you don't configure any access restrictions you create a public tor proxy. As this might result in people requesting illegal content via your IP you should configure some `blacklisted-words`. If a response content matches any of there words (checked with a boundary regex) the response is blocked.

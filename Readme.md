# zwiebelproxy

this is a small setup to access TOR services via a custom domain like the `tor2web` service. You can specify a domain, for example `onion.tld` and access a tor hidden service via `toraddress.onion.tld`. All you need to do is point a wildcard domain at this server.

Once the server receives an request it will check the domain and then forward it to the tor network and returns the reponse. This way you can setup some kind of a proxy into the tor network.

## Instructions

create a `.env` file with the required env variables or supply the parameters. View the `--help` for more information.

you can run `./start.sh` or use `docker compose up` to start the service.

To use it in production please use a http reverse proxy in front of this to handle all the TLS stuff and maybe also authentication.

Example `nginx.conf`:

```conf
server {
  listen 80;
  listen [::]:80;

  server_name onion.tld *.onion.tld;

  root /var/www/html;
  index index.html;

  access_log /var/log/nginx/access.log;
  error_log /var/log/nginx/error.log;

  location /.well-known {
    allow all;
    break;
  }

  location / {
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_pass http://localhost:8000;
  }
}

server {
  listen 443 ssl http2;
  listen [::]:443 ssl http2;
  server_name onion.tld *.onion.tld;
  root /var/www/html;
  index index.html;

  access_log /var/log/nginx/access.log;
  error_log /var/log/nginx/error.log;

  ssl_certificate /etc/letsencrypt/live/onion.tld/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/onion.tld/privkey.pem;

  location /.well-known {
    allow all;
    break;
  }

  location / {
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_pass http://localhost:8000;
  }
}
```
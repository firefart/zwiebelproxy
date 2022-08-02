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

  # uncomment and adapt for ip based ACLs
  # allow 8.8.8.8/32;
  # allow 10.0.0.0/8;
  # deny all;

  location / {
    proxy_read_timeout 5m; # this needs to be equal or higher than your configured timeout
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

  # uncomment and adapt for ip based ACLs
  # allow 8.8.8.8/32;
  # allow 10.0.0.0/8;
  # deny all;

  location / {
    proxy_read_timeout 5m; # this needs to be equal or higher than your configured timeout
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_pass http://localhost:8000;
  }
}
```

Example with [https://github.com/firefart/nginxreverseauth](https://github.com/firefart/nginxreverseauth) as an authentication proxy (the service needs to be running and configured for this to work):

```conf
server {
  listen 80;
  listen [::]:80;

  server_name onion.tld *.onion.tld;

  root /var/www/html;
  index index.html;

  access_log /var/log/nginx/zwiebelproxy_access.log;
  error_log /var/log/nginx/zwiebelproxy_error.log;

  location /.well-known {
    allow all;
    break;
  }

  location = /zwiebelproxy_auth {
    # only allow local requests to the auth endpoint
    # change if needed
    allow 127.0.0.0/8;
    allow 10.0.0.0/8;
    allow 172.16.0.0/12;
    allow 192.168.0.0/16;
    deny all;

    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URI $request_uri;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_pass http://localhost:8081;
  }

  location / {
    auth_request /zwiebelproxy_auth;
    proxy_read_timeout 5m; # this needs to be equal or higher than your configured timeout
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

  access_log /var/log/nginx/zwiebelproxy_access.log;
  error_log /var/log/nginx/zwiebelproxy_error.log;

  ssl_certificate /etc/letsencrypt/live/onion.tld/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/onion.tld/privkey.pem;

  location /.well-known {
    allow all;
    break;
  }

  location = /zwiebelproxy_auth {
    # only allow local requests to the auth endpoint
    # change if needed
    allow 127.0.0.0/8;
    allow 10.0.0.0/8;
    allow 172.16.0.0/12;
    allow 192.168.0.0/16;
    deny all;

    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URI $request_uri;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_pass http://localhost:8081;
  }

  location / {
    auth_request /zwiebelproxy_auth;
    proxy_read_timeout 5m; # this needs to be equal or higher than your configured timeout
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_pass http://localhost:8000;
  }
}
```

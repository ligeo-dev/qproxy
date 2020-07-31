## What is Qproxy ?

**Qproxy** is an open-source module which allows to easily manage queueing lines on a website, in case of intense web traffic spikes.
Qproxy guarantees the quality of user experience while suppressing the risk of complete unavailability of a website.
**It acts like a proxy** and only allows a certain number of sessions on a website, while others are put in queue.
When the number of authorized sessions is reached, users are welcomed on the website with a waiting message that refreshes automatically when the service is available.

## Why was it created ?

Qproxy was created for a client whose website is heavily solicited 3 days per year.
The idea was to offer **a quick and simple tool to avoid expensive hardware developments**, especially for such a short period of time.
Qproxy was also **designed to benefit users** and protect the quality of their online experience on a website.

## Getting started

### Building

Juste run "make build" to build the project.

### Configuration

| Parameter | Description |
| --- | --- |
| `addr` | the address to listen on |
| `cookie_name` | the name of the cookie used to store session ID |
| `session_refresh_interval` | interval, in seconds, between sessions expiration check |
| `timeout` | maximum duration of request processing |
| `trusted_proxies` | list of trusted proxies ips in front of QProxy (example: `[192.0.0.1, 10.0.0.0/8]`) |
| `whitelisted_ips` | list of whitelisted ips allowed to bypass session's check |
| `tls.cert_file` | proxy cert file  |
| `tls.key_file` | proxy key file |
| `queue.max_sessions` | maximum queued sessions, set to `0` to disable  |
| `queue.session_ttl` | queue session lifetime |
| `queue.template` | path to queue's html template |
| `queue.full_template` | path to saturation's html template |
| `api.addr` | api listen address |
| `api.tls.cert_file` | api cert file |
| `api.tls.key_file` | api key file |
| `api.username` | the login of the allowed user to access the `/api` endpoint, leave empty to disable  |
| `api.username` | the password of the allowed user to access the `/api` endpoint, leave empty to disable |
| `backends.{backend_name}.url` | backend url (example: `http://127.0.0.1:8080`) |
| `backends.{backend_name}.max_sessions` | maximum allowed sessions to backend |
| `backends.{backend_name}.session_ttl` | backend session lifetime |
| `backends.{backend_name}.weight` | the weight of the backend, defaults to `1` |
| `backends.{backend_name}.tls.insecure` | skip backend tls verify, defaults to `false` |

### Running

```
./qproxy -c {path_to_config_file.yaml}
```

## License & credits

This project is licensed under MIT license.

QProxy is maintained by [<span lang="fr">Empreinte Digitale</span>  (French)](http://empreintedigitale.fr).

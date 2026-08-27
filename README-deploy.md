# Deploying opentea on Debian with systemd

Standard FHS layout, no reverse proxy — the server listens directly on `TEA_LISTEN_ADDR`
(default `:8080`, or HTTPS if `TEA_TLS_CERT_FILE`/`TEA_TLS_KEY_FILE` are set — see step 4).
Unit files and example config are in `packaging/`.

By default this one listener serves everything — the consumer API (`/tea/v1` + `/files`) and the
admin surface (`/admin/v1` + `/admin/ui`) alike. Two independent knobs split that apart if your
deployment needs it (both optional, both documented inline in `packaging/opentea.conf.example`):
`TEA_ADMIN_LISTEN_ADDR` binds the admin surface to its own port/address (e.g. an internal-only
interface, keeping the consumer API on a public one), and `TEA_API_BASE_PATH` changes the path
prefix the consumer API itself answers on (default `/tea/v1`) — relevant if you're running
standalone, with no fronting reverse proxy, and need to answer literally at the `/v{version}`
path TEA's discovery spec has clients construct. Neither is covered further in this guide; see
`README.md`'s config table for details.

Configuration is layered: **built-in defaults < config file < environment variables**
(environment variables always win). Two independent ways to set values, pick whichever fits —
or mix them, since env vars override the file either way:

- **`/etc/opentea/opentea.conf`** (`packaging/opentea.conf.example`) — read directly by the
  `opentea` binary at startup, regardless of how it's launched. Override the path with
  `TEA_CONFIG_FILE`.
- **`/etc/opentea/opentea.env`** (`packaging/systemd/opentea.env.example`) — sourced by
  systemd's `EnvironmentFile=` *before* the process starts, becoming real environment
  variables. systemd-specific.

This guide uses `opentea.conf` since it's the simpler, systemd-independent option.

## 1. Build and install

On the target machine (or cross-compile and copy the binaries over):

```bash
cd /path/to/opentea
make build          # -> bin/opentea, bin/teaclient, bin/fixtures, bin/bundlecheck (as your normal user)
sudo make install    # -> /usr/local/bin (override with PREFIX=... or BINDIR=...)
```

Build and install are deliberately separate steps run as different users: `make build` must run
as your normal user (not root) since `go build` shells out to `git` to stamp VCS info, which
fails under `sudo` if the repo isn't owned by root (git's dubious-ownership check). `make
install` only copies the binaries already in `./bin` -- it errors out immediately, without
attempting a build, if you skip step one.

`make install` installs all four components (the server plus the reference client, fixtures,
and bundlecheck tools) -- only `opentea` itself needs the systemd unit below.

## 2. Create the group and user

```bash
sudo groupadd --system opentea
sudo useradd --system --gid opentea --home-dir /var/lib/opentea \
    --no-create-home --shell /usr/sbin/nologin opentea-server
```

`opentea-server`'s **primary** group is `opentea` — that's what makes it "belong to" the group
(`id opentea-server` will show `gid=...(opentea)`). No login shell, no home directory contents
(the data directory is created explicitly in the next step).

## 3. Create the data and config directories

```bash
sudo mkdir -p /var/lib/opentea /etc/opentea
sudo chown opentea-server:opentea /var/lib/opentea
sudo chmod 750 /var/lib/opentea
sudo chown root:opentea /etc/opentea
sudo chmod 750 /etc/opentea
```

## 4. Install the config file

```bash
sudo cp packaging/opentea.conf.example /etc/opentea/opentea.conf
sudo chown root:opentea /etc/opentea/opentea.conf
sudo chmod 640 /etc/opentea/opentea.conf
```

Edit `/etc/opentea/opentea.conf` — at minimum, set `TEA_ROOT_URL` to this server's real
hostname/scheme once you know it (it's used in discovery responses and generated file URLs),
and `TEA_ORG_NAME` if you want it shown in the admin GUI and `GET /admin/v1/stats`.

To enable HTTPS directly (instead of terminating TLS elsewhere), also set both
`TEA_TLS_CERT_FILE` and `TEA_TLS_KEY_FILE` in the file to real certificate/key paths readable
by `opentea-server` — setting only one of the two is a startup-time config error, not a silent
fallback to plain HTTP.

## 5. Install the systemd unit

```bash
sudo cp packaging/systemd/opentea.service /etc/systemd/system/opentea.service
sudo systemctl daemon-reload
```

## 6. Bootstrap the first admin user

This must run once, before starting the service, as the `opentea-server` user. Since
`/etc/opentea/opentea.conf` is read automatically at its default path, no extra environment
wiring is needed here — it'll open the same database the service will:

```bash
sudo -u opentea-server /usr/local/bin/opentea createadmin -username=admin -password='a-real-password'
```

## 7. Enable and start

```bash
sudo systemctl enable --now opentea.service
sudo systemctl status opentea.service
sudo journalctl -u opentea.service -f
```

## 8. Verify

```bash
curl http://localhost:8080/tea/v1/products
# log into /admin/ui/login with the admin user from step 6, or:
curl -c /tmp/cookies.txt -X POST http://localhost:8080/admin/ui/login -d 'username=admin&password=a-real-password'
curl -b /tmp/cookies.txt http://localhost:8080/admin/v1/stats
# {"startedAt":"...","orgName":"Acme Corp","products":0,...}
# startedAt confirms when the current process started (use it for uptime/monitoring);
# orgName reflects TEA_ORG_NAME if you set it in opentea.conf.
```

If you enabled TLS in step 4, use `https://` and port matches `TEA_LISTEN_ADDR` instead.

## Upgrading

```bash
sudo systemctl stop opentea.service
make build && sudo make install
sudo systemctl start opentea.service
```

The SQLite database and blob storage under `/var/lib/opentea` persist across upgrades and
restarts untouched.

## Uninstalling

```bash
sudo systemctl disable --now opentea.service
sudo rm /etc/systemd/system/opentea.service
sudo systemctl daemon-reload
sudo make uninstall                          # removes opentea/teaclient/fixtures from $(BINDIR)
sudo rm -rf /var/lib/opentea /etc/opentea    # deletes all data -- back up first if needed
sudo userdel opentea-server
sudo groupdel opentea
```

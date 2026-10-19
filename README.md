[English](/README.md) | [فارسی](/README.fa_IR.md) | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

<h1 align="center">Eternal</h1>

<p align="center">
  A self-hosted control panel for proxy and VPN servers: Xray-core, OpenVPN, IKEv2, L2TP, SSTP, OpenConnect, SSH tunnels, WireGuard-family protocols, a built-in shop, resellers and multi-node management.
</p>

<p align="center">
  <a href="https://www.gnu.org/licenses/gpl-3.0.en.html"><img src="https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true" alt="License"></a>
</p>

> [!NOTE]
> **Eternal is a fork of [3x-ui](https://github.com/MHSanaei/3x-ui)** by MHSanaei (itself a fork of the original X-UI project). It keeps the 3x-ui core and adds many features of its own, listed below. 3x-ui is licensed under GPL-3.0, and so is this fork: the full license text is in [LICENSE](/LICENSE) and the copyright of the original authors is preserved. Many thanks to the 3x-ui maintainers and contributors.

> [!IMPORTANT]
> This project is intended for personal use. Please do not use it for illegal purposes.

## Comparison with 3x-ui

The first rows are shared with 3x-ui; the rest exist only in Eternal. The comparison is against the 3x-ui version this fork branched from and may be outdated if upstream has added features since.

| Feature | Eternal | 3x-ui |
| --- | :---: | :---: |
| VLESS, VMess, Trojan, Shadowsocks, REALITY, XTLS | ✅ | ✅ |
| Hysteria2, TUIC v5, WireGuard, AmneziaWG, MTProto | ✅ | ✅ |
| Per-client quota, expiry, IP and device limits | ✅ | ✅ |
| Multi-node management | ✅ | ✅ |
| Subscription server (raw, JSON, Clash) | ✅ | ✅ |
| Telegram and Discord bots | ✅ | ✅ |
| REST API with scoped tokens | ✅ | ✅ |
| Routing rules, WARP and outbound chaining | ✅ | ✅ |
| SQLite and PostgreSQL | ✅ | ✅ |
| Fail2ban IP limits | ✅ | ✅ |
| 13 languages, dark and light themes, PWA | ✅ | ✅ |
| OpenVPN inbounds | ✅ | ❌ |
| IKEv2/IPsec and L2TP/IPsec inbounds | ✅ | ❌ |
| SSTP (Windows built-in VPN) and OpenConnect/AnyConnect | ✅ | ❌ |
| SSH tunnel accounts with metered traffic | ✅ | ❌ |
| Built-in shop: plans, orders, public purchase page | ✅ | ❌ |
| Card-to-card, Zarinpal, IDPay, crypto (NOWPayments) and Stripe payments | ✅ | ❌ |
| Customer wallet, discount codes, referral bonus | ✅ | ❌ |
| Reseller panels: separate isolated panels with their own login, address and quotas | ✅ | ❌ |
| Roles and per-admin scoping (RBAC), admin sessions | ✅ | ❌ |
| Panel IP allow-list, login lockout, captcha, new-IP/country alerts | ✅ | ❌ |
| Telegram one-time-code login, signed subscription links | ✅ | ❌ |
| MySQL / MariaDB backend | ✅ | ❌ |
| Encrypted and scheduled backups to local folder or S3 | ✅ | ❌ |
| Prometheus metrics and Grafana dashboard | ✅ | ❌ |
| Cloudflare clean-IP scanner, port/SNI rotation, blocked-port alerts | ✅ | ❌ |
| One-command node install and automatic node failover | ✅ | ❌ |
| Node monthly cap, cost tracking and speed test | ✅ | ❌ |
| Client templates, daily usage chart, instant disconnect | ✅ | ❌ |
| Import from Marzban, Marzneshin and Hiddify | ✅ | ❌ |
| Telegram trial accounts | ✅ | ❌ |
| sing-box, Outline, Surge, Quantumult X and Loon subscriptions | ✅ | ❌ |
| Short subscription links, rate limiting, fetch statistics | ✅ | ❌ |
| Webhooks, ntfy, Gotify, Slack and Bale notifications | ✅ | ❌ |
| Editable bot texts and weekly summary report | ✅ | ❌ |
| Shared-account detection and maintenance mode | ✅ | ❌ |
| Panel health check, usage report export, keyboard shortcuts | ✅ | ❌ |
| Decoy site for unknown visitors, audit log, branding settings | ✅ | ❌ |
| `eternal client` command line | ✅ | ❌ |

## What the fork adds

On top of everything 3x-ui provides (VLESS, VMess, Trojan, Shadowsocks, REALITY, Hysteria2, TUIC, AmneziaWG, MTProto, multi-node, subscriptions, Telegram and Discord bots, REST API), Eternal adds:

**More protocols**
- OpenVPN, IKEv2/IPsec (strongSwan), L2TP/IPsec, SSTP (Windows built-in VPN), OpenConnect / AnyConnect (ocserv)
- SSH tunnel accounts with panel-metered traffic, expiry and device limits
- Per-client traffic accounting, quota, expiry and instant disconnect for all of them

**Selling and billing**
- Plans, orders and a public purchase page that needs no panel login
- Card-to-card payments with admin approval, Zarinpal, IDPay, NOWPayments (crypto) and Stripe
- Customer wallet, discount codes and referral bonuses
- Reseller panels: separate isolated panels with their own login, address and quotas
- Client transfer between admins

**Operations**
- Roles and per-admin scoping (RBAC), admin sessions, panel IP allow-list, login lockout, login captcha
- New-IP / new-country login alerts, Telegram one-time-code login, signed subscription links
- Encrypted backups, scheduled backups to a local folder or any S3-compatible bucket, split uploads for large files
- MySQL / MariaDB support next to SQLite and PostgreSQL
- Prometheus metrics endpoint and a ready Grafana dashboard
- Panel health check, maintenance mode, shared-account detection, keyboard shortcuts

**Anti-censorship and nodes**
- Cloudflare clean-IP scanner, blocked-port alerts, port and REALITY SNI rotation
- One-click Iran routing template
- One-command node install and automatic subscription failover to a backup node
- Node speed test, monthly traffic cap and cost tracking per node

**Clients and subscriptions**
- Client templates, daily usage history, import from Marzban, Marzneshin and Hiddify
- Telegram trial accounts, sing-box / Outline / Surge / Quantumult X / Loon subscription formats
- Short subscription links, rate limiting and fetch statistics
- `eternal client` command line for scripting

**Notifications**
- Webhooks, ntfy, Gotify, Slack, Bale, editable bot texts, weekly summary, quota and expiry warnings

**Interface**
- A redesigned, minimal interface with a new login page, dashboard widgets, branding settings, audit log, a read-only sponsors page and a decoy site for unknown visitors
- 13 UI languages with dark and light themes, RTL included

## Install

The installer and the update script download release archives from this repository's GitHub releases (built by the release workflow), so publish a release first, or build from source.

```bash
bash <(curl -Ls https://raw.githubusercontent.com/rad1in/eternal/main/install.sh)
```

During installation a random username, password and access path are generated. Run `eternal` afterwards to open the management menu.

### Build from source

Requirements: Go (see `go.mod`), a C compiler (the SQLite driver needs CGo), Node 26 for the frontend.

```bash
cd frontend && npm ci && npm run build && cd ..
go build -o eternal .
```

### Docker

```bash
docker compose up -d
```

Fail2ban inside the image bans with `iptables`; if you use `docker run` instead of compose, add `--cap-add=NET_ADMIN --cap-add=NET_RAW`.

## Databases

SQLite (default), PostgreSQL and MySQL/MariaDB. Select with environment variables, which the installer writes to `/etc/default/eternal`:

```
XUI_DB_TYPE=postgres        # sqlite | postgres | mysql
XUI_DB_DSN=postgres://user:password@127.0.0.1:5432/db?sslmode=disable
```

`eternal migrate-db --dsn "<dsn>"` copies an existing SQLite database into PostgreSQL or MySQL.

## Sponsors

Sponsors are one JSON file in a GitHub repository that only the project owner
can edit; there is no sponsor setting, page or API in any panel. Every panel,
including the panels of hosted resellers, reads

```
https://raw.githubusercontent.com/rad1in/sponsors/main/sponsors.json
```

and the logos from `logos/` next to it. Panels cache the file for an hour
(10 minutes after a failed fetch, keeping the last good list), re-check each
entry's `from` and `until` on every request, and show no sponsors, without any
error, when the file is missing or unreachable.

To publish a sponsor, add an entry to that file and its logo to `logos/`. The
format, with every field explained in the [sponsors reference](docs/content/docs/en/reference/sponsors.mdx):

```json
{
  "contact": "https://t.me/example",
  "sponsors": [
    {
      "id": "acme-2026-10",
      "name": "Acme VPS",
      "enable": true,
      "slots": ["dashboard", "page", "login"],
      "from": "2026-10-01T00:00:00Z",
      "until": "2026-11-01T00:00:00Z",
      "logo": "acme.png",
      "title": {
        "en": "Acme VPS: 20% off",
        "fa": "Acme VPS: ۲۰٪ تخفیف"
      },
      "text": {
        "en": "Fast servers for your panel. Use code ETERNAL at checkout.",
        "fa": "سرورهای سریع برای پنل شما. هنگام خرید از کد ETERNAL استفاده کنید."
      },
      "link": "https://acme.example/?utm_source=eternal"
    }
  ]
}
```

## Documentation

- Architecture and layering rules: [`docs/architecture.md`](docs/architecture.md)
- Contributing guide: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- API reference: served inside the panel and generated into `docs/content/docs/en/reference/api/`
- Upstream documentation for the shared core: [docs.sanaei.dev](https://docs.sanaei.dev)

## Languages

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## License and credits

Eternal is free software under the **GNU GPL v3**. It is based on:

- [3x-ui](https://github.com/MHSanaei/3x-ui) by MHSanaei and contributors (GPL-3.0)
- [X-UI](https://github.com/vaxilu/x-ui) by vaxilu, the project 3x-ui was forked from (GPL-3.0)
- [Xray-core](https://github.com/XTLS/Xray-core) by the XTLS team (MPL-2.0)

Routing rule sets used by the Iran and Russia templates come from [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) and [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat) (GPL-3.0).

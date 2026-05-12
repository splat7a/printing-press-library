---
name: pp-meta-ads
description: "Printing Press CLI for Meta Ads raw snapshots. Pulls SabeenManekia-compatible Meta Graph JSON files without passing tokens on argv."
argument-hint: "snapshot pull --out <dir> | doctor --json | agent-context --pretty | install cli"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      env: ["META_ACCESS_TOKEN", "META_AD_ACCOUNT_ID", "META_BUSINESS_ID"]
      bins:
        - meta-ads-pp-cli
    envVars:
      - name: META_ACCESS_TOKEN
        required: true
        description: "Meta Graph system-user access token. Read from env only; never pass as argv."
      - name: META_AD_ACCOUNT_ID
        required: true
        description: "Meta ad account ID, for example act_123."
      - name: META_BUSINESS_ID
        required: true
        description: "Meta business ID."
      - name: META_GRAPH_API_VERSION
        required: false
        description: "Meta Graph API version. Defaults to v21.0."
    install:
      - kind: go
        bins: [meta-ads-pp-cli]
        module: github.com/mvanhorn/printing-press-library/library/marketing/meta-ads/cmd/meta-ads-pp-cli
---

# Meta Ads — Printing Press CLI

Use `meta-ads-pp-cli` to pull read-only Meta Ads raw JSON snapshots into the SabeenManekia-compatible staging layout.

## Safe invocation

Set credentials through environment variables only:

```bash
export META_ACCESS_TOKEN=...
export META_AD_ACCOUNT_ID=act_...
export META_BUSINESS_ID=...
meta-ads-pp-cli snapshot pull --out raw/meta
```

Never put access tokens in argv. The CLI uses an `Authorization: Bearer` header, strips `access_token` query params from paginated URLs, and redacts token material in HTTP errors before truncating output.

## Primary commands

- `meta-ads-pp-cli snapshot pull --out raw/meta` — write all raw JSON files.
- `meta-ads-pp-cli doctor --json` — verify required environment variables without printing secret values.
- `meta-ads-pp-cli agent-context --pretty` — show the automation contract and expected raw filenames.

## Raw files

`snapshot pull` writes the legacy raw contract files consumed by SabeenManekia downstream code:

- `business.json`
- `ad_account.json`
- `campaigns.json`
- `adsets.json`
- `ads.json`
- `creatives.json`
- `insights_campaigns_90d.json`
- `insights_adsets_90d.json`
- `insights_daily_90d.json`
- `custom_audiences.json`
- `pages.json`
- `instagram_accounts.json`
- `catalogs.json`

On endpoint failure, existing good JSON is left in place; otherwise an empty list is written for that file.

## Page-token rule

Page access tokens are not requested. If Graph returns an `access_token` field on a page object anyway, the CLI removes it before writing `pages.json`.

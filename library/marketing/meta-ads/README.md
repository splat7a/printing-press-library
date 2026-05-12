# Meta Ads Printing Press CLI

`meta-ads-pp-cli` pulls Meta Graph raw JSON snapshots while preserving the SabeenManekia downstream raw-file contract.

## Credentials

Credentials are read from environment variables only. Do not pass tokens on the command line.

Required:

- `META_ACCESS_TOKEN`
- `META_AD_ACCOUNT_ID`
- `META_BUSINESS_ID`

Optional:

- `META_GRAPH_API_VERSION` (default: `v21.0`)
- `META_GRAPH_BASE_URL` (test override; default: `https://graph.facebook.com`)

## Snapshot pull

```bash
meta-ads-pp-cli snapshot pull --out raw/meta
```

The command writes:

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

Each file is written atomically via a temporary file and replace. If a Meta endpoint fails, the CLI preserves an existing non-empty/non-`null` valid JSON file; if no usable prior file exists, it writes an empty list. The CLI sends the Meta token with an `Authorization: Bearer` header, strips `access_token` from paginated URLs, and redacts token material in HTTP errors before truncating output. Page access tokens are not requested. Any returned `access_token` fields are removed before writing `pages.json`, so page access tokens are not persisted in raw JSON.

## Agent helpers

```bash
meta-ads-pp-cli doctor --json
meta-ads-pp-cli agent-context --pretty
```

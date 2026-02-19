# Monitoring Notification Message Template

This document explains how to customize monitoring notification text and which template arguments are supported.

## Where it is configured

The template is configured in monitoring notification channel settings (channel type `telegram`) in the `Template` field.

Template behavior:

- If template is empty, the default system message is sent.
- If template is set, `{message}` is replaced with the default system message.
- If `{message}` is not present, only static template text is sent.

## Supported arguments

Only one argument is supported:

- `{message}` - full default notification message (title, monitor name, target, error/latency, time, footer).

Other placeholders (`{name}`, `{url}`, `{error}`, etc.) are not parsed.

## What `{message}` contains

Content depends on event type:

- `down`:
- monitor down title
- monitor name
- target (`URL` or `host:port`)
- error reason (`Error: ...`), including HTTP status or monitor error text
- latency (if available)
- time
- footer `Berkut SCC`

- `up`:
- monitor recovered title
- monitor name
- target
- latency (if available)
- time
- footer `Berkut SCC`

- `tls_expiring`:
- TLS expiring title
- monitor name
- target
- expiration date
- days left
- time
- footer

## Template examples

Default system message:

```text
{message}
```

Prefix + system message:

```text
[PROD][SOC]
{message}
```

System message + suffix:

```text
{message}

#noc
```

Static-only message:

```text
Monitor alert triggered. See details in Berkut SCC.
```

## Recommendation

Keep `{message}` in template to preserve all default details.

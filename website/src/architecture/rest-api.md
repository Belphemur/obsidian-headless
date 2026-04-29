---
title: REST API
---

# REST API

[[toc]]

## Overview

The Obsidian REST API provides HTTP endpoints for authentication, vault management, and publish operations. The headless client communicates with these endpoints to sync data, manage sites, and upload content.

::: tip
All API requests are `POST` with `Content-Type: application/json` unless otherwise noted.
:::

## Base URLs

| Service      | Base URL                              |
| ------------ | ------------------------------------- |
| Obsidian API | `https://api.obsidian.md`             |
| Publish API  | `https://publish.obsidian.md`         |
| Host API     | `https://<site-host>` (per-site)      |

The Host API base URL is determined by the publish site configuration returned from the Publish API.

## Authentication

All endpoints accept authentication via the `token` field in the JSON request body. Tokens are obtained via the sign-in endpoint and should be included in every subsequent request.

::: warning
Store tokens securely. They grant full access to your Obsidian account, vaults, and publish sites.
:::

## Common Response Format

All responses are JSON objects. Errors include an `error` or `msg` field:

```json
{
  "error": "Human-readable error message"
}
```

## Obsidian API Endpoints

### POST `/user/signin`

Authenticate with email and password.

**Request:**

```json
{
  "email": "user@example.com",
  "password": "secret",
  "mfa": "123456"
}
```

| Field      | Type   | Required | Description                      |
| ---------- | ------ | -------- | -------------------------------- |
| `email`    | string | yes      | Account email                    |
| `password` | string | yes      | Account password                 |
| `mfa`      | string | no       | Two-factor authentication code   |

**Response:**

```json
{
  "token": "abc123...",
  "name": "User Name",
  "email": "user@example.com"
}
```

### POST `/user/signout`

Sign out and invalidate the token.

**Request:**

```json
{
  "token": "abc123..."
}
```

### POST `/user/info`

Get the current user's profile.

**Request:**

```json
{
  "token": "abc123..."
}
```

**Response:**

```json
{
  "uid": "user-id",
  "name": "User Name",
  "email": "user@example.com",
  "mfa": false,
  "license": "catalyst",
  "credit": 0,
  "discount": 0
}
```

### POST `/vault/regions`

List available server regions for vault creation.

**Request:**

```json
{
  "token": "abc123...",
  "host": "optional-host"
}
```

**Response:**

```json
{
  "regions": [
    { "id": "us-east", "name": "US East" },
    { "id": "eu-west", "name": "EU West" }
  ]
}
```

### POST `/vault/list`

List all vaults accessible to the user.

**Request:**

```json
{
  "token": "abc123...",
  "supported_encryption_version": 3
}
```

**Response:**

```json
{
  "vaults": [
    {
      "id": "vault-id",
      "uid": "vault-uid",
      "name": "My Vault",
      "password": "key-hash",
      "salt": "hex-salt",
      "created": 1700000000,
      "host": "sync-1.obsidian.md",
      "size": 52428800,
      "encryption_version": 3
    }
  ],
  "shared": []
}
```

### POST `/vault/create`

Create a new remote vault.

**Request:**

```json
{
  "token": "abc123...",
  "name": "My Vault",
  "keyhash": "hex-key-hash",
  "salt": "hex-salt",
  "region": "us-east",
  "encryption_version": 3
}
```

**Response:**

```json
{
  "id": "new-vault-id",
  "name": "My Vault"
}
```

### POST `/vault/access`

Validate access to a vault with the provided key hash.

**Request:**

```json
{
  "token": "abc123...",
  "uid": "vault-uid",
  "keyhash": "hex-key-hash",
  "host": "sync-1.obsidian.md",
  "supported_encryption_version": 3
}
```

**Response:**

```json
{
  "status": "ok"
}
```

## Publish API Endpoints

### POST `/publish/list`

List all publish sites.

**Request:**

```json
{
  "token": "abc123..."
}
```

**Response:**

```json
{
  "sites": [
    {
      "id": "site-id",
      "slug": "my-site",
      "host": "publish-1.obsidian.md",
      "created": 1700000000
    }
  ],
  "shared": []
}
```

### POST `/publish/create`

Create a new publish site.

**Request:**

```json
{
  "token": "abc123..."
}
```

**Response:**

```json
{
  "id": "new-site-id",
  "host": "publish-1.obsidian.md"
}
```

## Host API Endpoints

These endpoints use the per-site host URL returned by the Publish API.

### POST `/api/slug`

Set the slug for a publish site.

**URL:** `https://publish.obsidian.md/api/slug`

**Request:**

```json
{
  "token": "abc123...",
  "id": "site-id",
  "host": "publish-1.obsidian.md",
  "slug": "my-site"
}
```

### POST `/api/slugs`

Get slug mappings for sites.

**URL:** `https://publish.obsidian.md/api/slugs`

**Request:**

```json
{
  "token": "abc123...",
  "ids": ["site-id-1", "site-id-2"]
}
```

### POST `/api/list`

List published files for a site.

**URL:** `https://<site-host>/api/list`

**Request:**

```json
{
  "token": "abc123...",
  "id": "site-id",
  "version": 2
}
```

**Response:**

```json
{
  "files": [
    {
      "path": "notes/hello.md",
      "hash": "abc123...",
      "size": 1024
    }
  ]
}
```

### POST `/api/upload`

Upload a file to the publish site.

**URL:** `https://<site-host>/api/upload`

::: warning
This endpoint sends raw binary data instead of JSON. Metadata is provided via custom headers.
:::

**Headers:**

| Header         | Value                        |
| -------------- | ---------------------------- |
| `Content-Type` | `application/octet-stream`   |
| `obs-token`    | Authentication token         |
| `obs-id`       | Site ID                      |
| `obs-path`     | URL-encoded file path        |
| `obs-hash`     | Content hash                 |

**Example request:**

```http
POST /api/upload HTTP/1.1
Host: publish-1.obsidian.md
Content-Type: application/octet-stream
obs-token: abc123...
obs-id: site-id
obs-path: notes%2Fhello.md
obs-hash: def456...

<raw file bytes>
```

### POST `/api/remove`

Remove a file from the publish site.

**URL:** `https://<site-host>/api/remove`

**Request:**

```json
{
  "token": "abc123...",
  "id": "site-id",
  "path": "notes/hello.md"
}
```

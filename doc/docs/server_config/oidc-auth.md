---
sidebar_position: 11
description: Enabling OpenID Connect (OIDC) as an authentication provider.
---

# OpenID Connect (OIDC) Authentication

Watcharr can let users log in with any OpenID Connect provider (Authentik, Authelia, Keycloak, Pocket ID, Google, etc). It uses the authorization code flow with PKCE.

Users logging in via OIDC for the first time have a Watcharr account created for them automatically.

## Setup Steps

1. In your OIDC provider, create a new client/application for Watcharr.

   - **Redirect URI:** `https://<your-watcharr-url>/oidc_callback` (for example `https://watcharr.example.com/oidc_callback`).
   - **Scopes:** `openid`, `profile` and `email`.
   - Note down the **Client ID** and **Client Secret** (a public client without a secret also works).

2. Stop Watcharr and add an `OIDC` section to your `watcharr.json` config file (found in your data folder):

   ```json
   "OIDC": {
     "enabled": true,
     "issuer": "https://auth.example.com/application/o/watcharr/",
     "clientId": "your-client-id",
     "clientSecret": "your-client-secret",
     "buttonName": "Authentik"
   }
   ```

3. Start Watcharr. A **Continue with Authentik** button (using your `buttonName`) will now be shown on the login page.

## Options

| Option          | Required | Description                                                                                                                               |
| --------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`       | Yes      | Turn OIDC login on or off.                                                                                                                |
| `issuer`        | Yes      | Your provider's issuer URL. Watcharr reads `<issuer>/.well-known/openid-configuration` from it, so it must match the provider's issuer exactly. |
| `clientId`      | Yes      | Client ID from your provider.                                                                                                             |
| `clientSecret`  | No       | Client secret from your provider. Leave out for public clients.                                                                           |
| `scopes`        | No       | Scopes to request. Defaults to `["openid", "profile", "email"]`.                                                                          |
| `usernameClaim` | No       | Claim used as the username for new users. Defaults to `preferred_username`, falling back to `name`, `email` then `sub`.                   |
| `buttonName`    | No       | Name shown on the login button. Defaults to `Single Sign-On`.                                                                             |

## Notes

- Users are linked to their OIDC account by the provider's issuer and their `sub` (subject) claim, not their username. Renaming a user in your provider won't create a new Watcharr account, but **changing the `issuer` will**, since users from a different issuer are treated as different users.
- If two OIDC users have the same username, the second one gets a number added to the end (e.g. `bob2`).
- To make an OIDC user an admin, follow the [restoring admin](./restoring-admin) guide or have an existing admin change their permissions.

# Workspace Insights Demo

A full-stack app demonstrating **OAuth2 integration with Google** — how your backend connects to a third-party provider on behalf of your client and fetches their data.

## Architecture

```
React Frontend (port 5173)
    │
    ▼
Go Backend (port 3001)
    │  ← stores OAuth tokens per user (in-memory)
    ▼
Google APIs (userinfo, Admin SDK)
```

## Prerequisites

### 1. Create Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a new project (e.g., "Workspace Insights Demo")
3. Go to **APIs & Services → Library** and enable:
   - **Google People API**
4. Go to **APIs & Services → OAuth consent screen**
   - Choose **External** user type
   - Fill in app name, email
   - Add scopes: `openid`, `profile`, `email`
   - Add your email as a test user
5. Go to **APIs & Services → Credentials → Create Credentials → OAuth Client ID**
   - Application type: **Web application**
   - Authorized redirect URIs: `http://localhost:3001/auth/google/callback`
6. Copy the **Client ID** and **Client Secret**

### 2. Configure Environment

```bash
cd backend
cp .env.example .env
# Edit .env — paste your Client ID and Client Secret
```

## Running

### Backend (Terminal 1)
```bash
cd backend
go mod tidy
go run .
```

### Frontend (Terminal 2)
```bash
cd frontend
npm install
npm run dev
```

Open **http://localhost:5173**

## Demo Flow

1. Enter any name and click **Login** (demo auth — no real password)
2. Click **Connect Google** → browser redirects to Google consent screen
3. Grant permission → redirected back, tokens stored in backend
4. Dashboard shows your Google profile fetched via stored OAuth tokens
5. Click **Disconnect** to revoke access and delete tokens

## Key Concepts Demonstrated

| Concept | Where |
|---------|-------|
| OAuth2 Authorization Code Flow | `main.go` → `handleGoogleAuth` + `handleGoogleCallback` |
| Token exchange (code → access+refresh tokens) | `main.go` → `handleGoogleCallback` |
| Storing tokens per tenant | `main.go` → `tokenStore` (in-memory map) |
| Using tokens to call provider APIs | `main.go` → `handleGoogleProfile` |
| Automatic token refresh | `main.go` → `googleHTTPClient()` uses `oauth2.TokenSource` |
| CSRF protection with state parameter | `main.go` → `oauthStates` map |
| Token revocation on disconnect | `main.go` → `handleGoogleDisconnect` |

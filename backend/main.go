package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// =============================================================================
// IN-MEMORY STORES
//
// In production you'd use a database (Postgres, DynamoDB, etc.) and encrypt
// tokens at rest. This demo keeps everything in memory for clarity.
// =============================================================================

type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

var (
	// sessions maps a session cookie value → *User
	sessions sync.Map

	// tokenStore maps "userID:provider" → *oauth2.Token
	// This is the key store: your backend holds tokens that let it
	// talk to Google (or any provider) on behalf of each client.
	tokenStore sync.Map

	// oauthStates maps a random state string → userID for CSRF protection
	oauthStates sync.Map

	googleOAuth *oauth2.Config
	frontendURL string

	// ClickUp uses a simpler OAuth2 — no refresh tokens, no scopes.
	// The access token is long-lived.
	clickupClientID     string
	clickupClientSecret string
	clickupRedirectURI  = "http://localhost:3001/auth/clickup/callback"
)

func main() {
	_ = godotenv.Load()

	frontendURL = envOrDefault("FRONTEND_URL", "http://localhost:5173")

	// Configure the OAuth2 client.
	// client_id and client_secret come from Google Cloud Console.
	// These identify YOUR APPLICATION to Google.
	googleOAuth = &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  "http://localhost:3001/auth/google/callback",
		Scopes: []string{
			"openid",
			"https://www.googleapis.com/auth/userinfo.profile",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/drive.metadata.readonly",
			"https://www.googleapis.com/auth/calendar.readonly",
			"https://www.googleapis.com/auth/gmail.readonly",
		},
		Endpoint: google.Endpoint,
	}

	if googleOAuth.ClientID == "" || googleOAuth.ClientSecret == "" {
		log.Println("WARNING: GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET not set.")
	}

	clickupClientID = os.Getenv("CLICKUP_CLIENT_ID")
	clickupClientSecret = os.Getenv("CLICKUP_CLIENT_SECRET")
	if clickupClientID == "" || clickupClientSecret == "" {
		log.Println("WARNING: CLICKUP_CLIENT_ID or CLICKUP_CLIENT_SECRET not set.")
	}

	mux := http.NewServeMux()

	// --- App auth (simplified for demo — no passwords) ---
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/logout", handleLogout)
	mux.HandleFunc("/api/me", handleMe)

	// --- Google OAuth2 flow ---
	mux.HandleFunc("/auth/google", handleGoogleAuth)
	mux.HandleFunc("/auth/google/callback", handleGoogleCB)
	mux.HandleFunc("/api/google/status", handleGoogleStatus)
	mux.HandleFunc("/api/google/disconnect", handleGoogleDisconnect)

	// --- Google data ---
	mux.HandleFunc("/api/google/profile", handleGoogleProfile)
	mux.HandleFunc("/api/google/drive", handleGoogleDrive)
	mux.HandleFunc("/api/google/calendar", handleGoogleCalendar)
	mux.HandleFunc("/api/google/gmail", handleGoogleGmail)

	// --- ClickUp OAuth2 flow ---
	mux.HandleFunc("/auth/clickup", handleClickUpAuth)
	mux.HandleFunc("/auth/clickup/callback", handleClickUpCB)
	mux.HandleFunc("/api/clickup/status", handleClickUpStatus)
	mux.HandleFunc("/api/clickup/disconnect", handleClickUpDisconnect)

	// --- ClickUp data ---
	mux.HandleFunc("/api/clickup/profile", handleClickUpProfile)
	mux.HandleFunc("/api/clickup/workspaces", handleClickUpWorkspaces)
	mux.HandleFunc("/api/clickup/tasks", handleClickUpTasks)

	port := envOrDefault("PORT", "3001")
	log.Printf("Backend running at http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, withCORS(mux)))
}

// =============================================================================
// CORS MIDDLEWARE
// Allows the React frontend (port 5173) to call the backend (port 3001).
// =============================================================================

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", frontendURL)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// APP AUTH HANDLERS
// Simplified demo auth: user provides a name, gets a session cookie.
// In production this would be proper auth (passwords, SSO, etc.).
// =============================================================================

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httpError(w, http.StatusBadRequest, "name is required")
		return
	}

	user := &User{
		ID:        randomHex(16),
		Name:      body.Name,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	sessionID := randomHex(32)
	sessions.Store(sessionID, user)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	writeJSON(w, http.StatusOK, user)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if c, err := r.Cookie("session"); err == nil {
		sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: "", Path: "/", MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// =============================================================================
// GOOGLE OAUTH2 HANDLERS
//
// This is the heart of the demo. It shows how your backend:
//   1. Redirects the client to Google's consent screen
//   2. Receives the authorization code via callback
//   3. Exchanges the code for access + refresh tokens
//   4. Stores tokens so it can call Google APIs later
// =============================================================================

// STEP 1: Initiate OAuth — redirect user's browser to Google.
//
// When the client clicks "Connect Google", the frontend navigates to this endpoint.
// We generate a Google consent URL and redirect the browser there.
//
// What the user sees: Google's consent screen asking "Allow [Your App] to access your data?"
func handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		http.Redirect(w, r, frontendURL+"?error=not_logged_in", http.StatusTemporaryRedirect)
		return
	}

	// Random state for CSRF protection — we verify this in the callback.
	state := randomHex(16)
	oauthStates.Store(state, user.ID)

	// Build the consent URL. Key parameters:
	//   - access_type=offline  → we get a refresh_token (needed for background fetching)
	//   - prompt=consent       → always show consent (ensures we get refresh_token)
	consentURL := googleOAuth.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	log.Printf("[OAuth] Redirecting user %s to Google consent screen", user.ID[:8])
	http.Redirect(w, r, consentURL, http.StatusTemporaryRedirect)
}

// STEP 2: OAuth callback — Google redirects back here with an authorization code.
//
// Flow:
//   Browser → Google consent → user clicks "Allow" → Google redirects to this URL
//   URL looks like: /auth/google/callback?code=4/0AX4XfWh...&state=abc123
//
// We then exchange the code for tokens (server-to-server, no browser involved).
func handleGoogleCB(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	userIDVal, ok := oauthStates.LoadAndDelete(state)
	if !ok {
		log.Println("[OAuth] Invalid or expired state parameter")
		http.Redirect(w, r, frontendURL+"?error=invalid_state", http.StatusTemporaryRedirect)
		return
	}
	userID := userIDVal.(string)

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		log.Printf("[OAuth] Google returned error: %s", errParam)
		http.Redirect(w, r, frontendURL+"?error="+errParam, http.StatusTemporaryRedirect)
		return
	}

	// EXCHANGE: authorization code → access_token + refresh_token
	//
	// Under the hood this makes a POST request from your backend to Google:
	//   POST https://oauth2.googleapis.com/token
	//   Body: grant_type=authorization_code&code=...&client_id=...&client_secret=...
	//
	// Google verifies the code and returns:
	//   { "access_token": "ya29...", "refresh_token": "1//0g...", "expires_in": 3600 }
	code := r.URL.Query().Get("code")
	token, err := googleOAuth.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("[OAuth] Token exchange failed: %v", err)
		http.Redirect(w, r, frontendURL+"?error=token_exchange_failed", http.StatusTemporaryRedirect)
		return
	}

	// STORE the token for this user.
	// Now your backend can call Google APIs on behalf of this client at any time,
	// even when they're not online — using the refresh_token to get new access_tokens.
	tokenStore.Store(userID+":google", token)

	log.Printf("[OAuth] SUCCESS — tokens stored for user %s", userID[:8])
	log.Printf("[OAuth]   access_token:  %s...", token.AccessToken[:20])
	log.Printf("[OAuth]   refresh_token: %s...", token.RefreshToken[:10])
	log.Printf("[OAuth]   expires_at:    %s", token.Expiry.Format(time.RFC3339))

	http.Redirect(w, r, frontendURL+"?connected=google", http.StatusTemporaryRedirect)
}

// Check if the current user has connected Google.
func handleGoogleStatus(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	_, connected := tokenStore.Load(user.ID + ":google")

	result := map[string]any{
		"connected": connected,
		"provider":  "google",
	}

	if connected {
		token := getToken(user.ID, "google")
		result["expires_at"] = token.Expiry.Format(time.RFC3339)
		result["has_refresh_token"] = token.RefreshToken != ""
	}

	writeJSON(w, http.StatusOK, result)
}

// Disconnect: remove stored tokens and optionally revoke at Google.
func handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	token := getToken(user.ID, "google")
	if token != nil {
		// Best practice: revoke the token at Google so it can't be used even if leaked
		revokeURL := "https://oauth2.googleapis.com/revoke?token=" + token.AccessToken
		resp, err := http.Get(revokeURL)
		if err == nil {
			resp.Body.Close()
			log.Printf("[OAuth] Revoked token at Google for user %s", user.ID[:8])
		}
	}

	tokenStore.Delete(user.ID + ":google")
	log.Printf("[OAuth] Disconnected Google for user %s", user.ID[:8])

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// =============================================================================
// GOOGLE DATA HANDLERS
//
// These show how your backend fetches data from Google USING THE STORED TOKENS.
// The client doesn't need to be online. Your backend can do this in background
// jobs, cron tasks, etc.
// =============================================================================

// Fetch the connected user's Google profile.
//
// Under the hood:
//   GET https://www.googleapis.com/oauth2/v2/userinfo
//   Headers: Authorization: Bearer ya29.a0AfH6SM...
//
// If the access_token is expired, the oauth2 package automatically uses the
// refresh_token to get a new one — the client never knows this happened.
func handleGoogleProfile(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	client := googleHTTPClient(user.ID)
	if client == nil {
		httpError(w, http.StatusBadRequest, "google not connected — click Connect Google first")
		return
	}

	log.Printf("[API] Fetching Google profile for user %s", user.ID[:8])

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Printf("[API] Failed to fetch profile: %v", err)
		httpError(w, http.StatusBadGateway, "failed to call Google API")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[API] Google returned %d: %s", resp.StatusCode, string(body))
		httpError(w, resp.StatusCode, "Google API error: "+string(body))
		return
	}

	var profile map[string]any
	json.Unmarshal(body, &profile)

	// Wrap the response with metadata so the frontend can show what happened
	writeJSON(w, http.StatusOK, map[string]any{
		"data": profile,
		"_meta": map[string]any{
			"google_api_url": "https://www.googleapis.com/oauth2/v2/userinfo",
			"http_method":    "GET",
			"description":    "Your backend made this HTTP request to Google using the stored OAuth token",
		},
	})
}

// Fetch recent files from Google Drive.
// API: https://developers.google.com/drive/api/v3/reference/files/list
func handleGoogleDrive(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	client := googleHTTPClient(user.ID)
	if client == nil {
		httpError(w, http.StatusBadRequest, "google not connected")
		return
	}

	apiURL := "https://www.googleapis.com/drive/v3/files?" + url.Values{
		"pageSize": {"15"},
		"fields":   {"files(id,name,mimeType,modifiedTime,size,owners,webViewLink)"},
		"orderBy":  {"modifiedTime desc"},
	}.Encode()

	data, err := callGoogleAPI(client, apiURL)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"_meta": map[string]any{
			"google_api_url": "https://www.googleapis.com/drive/v3/files",
			"http_method":    "GET",
			"scope_used":     "drive.metadata.readonly",
			"description":    "Lists recent files from the user's Google Drive",
		},
	})
}

// Fetch upcoming calendar events.
// API: https://developers.google.com/calendar/api/v3/reference/events/list
func handleGoogleCalendar(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	client := googleHTTPClient(user.ID)
	if client == nil {
		httpError(w, http.StatusBadRequest, "google not connected")
		return
	}

	apiURL := "https://www.googleapis.com/calendar/v3/calendars/primary/events?" + url.Values{
		"maxResults":   {"15"},
		"timeMin":      {time.Now().Format(time.RFC3339)},
		"orderBy":      {"startTime"},
		"singleEvents": {"true"},
	}.Encode()

	data, err := callGoogleAPI(client, apiURL)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"_meta": map[string]any{
			"google_api_url": "https://www.googleapis.com/calendar/v3/calendars/primary/events",
			"http_method":    "GET",
			"scope_used":     "calendar.readonly",
			"description":    "Lists upcoming events from the user's primary Google Calendar",
		},
	})
}

// Fetch Gmail profile stats (total messages, threads, email) and labels.
// API: https://developers.google.com/gmail/api/reference/rest/v1/users/getProfile
func handleGoogleGmail(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	client := googleHTTPClient(user.ID)
	if client == nil {
		httpError(w, http.StatusBadRequest, "google not connected")
		return
	}

	profileData, err := callGoogleAPI(client, "https://www.googleapis.com/gmail/v1/users/me/profile")
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}

	labelsData, _ := callGoogleAPI(client, "https://www.googleapis.com/gmail/v1/users/me/labels")

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"profile": profileData,
			"labels":  labelsData,
		},
		"_meta": map[string]any{
			"google_api_urls": []string{
				"https://www.googleapis.com/gmail/v1/users/me/profile",
				"https://www.googleapis.com/gmail/v1/users/me/labels",
			},
			"http_method": "GET",
			"scope_used":  "gmail.readonly",
			"description": "Fetches Gmail profile stats and label list",
		},
	})
}

// callGoogleAPI is a helper that calls a Google API endpoint and returns parsed JSON.
func callGoogleAPI(client *http.Client, apiURL string) (map[string]any, error) {
	log.Printf("[API] GET %s", apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[API] Google returned %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("Google API returned %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	json.Unmarshal(body, &result)
	return result, nil
}

// =============================================================================
// CLICKUP OAUTH2 HANDLERS
//
// ClickUp's OAuth is simpler than Google's:
//   - No scopes (you get access to everything the user has)
//   - No refresh tokens (access token is long-lived)
//   - Token exchange uses JSON body instead of form-encoded
//
// This demonstrates that every provider has slightly different OAuth
// implementations, but the PATTERN is always the same:
//   redirect → consent → callback → exchange code → store token → call APIs
// =============================================================================

// STEP 1: Redirect to ClickUp's authorization page.
func handleClickUpAuth(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		http.Redirect(w, r, frontendURL+"?error=not_logged_in", http.StatusTemporaryRedirect)
		return
	}

	state := randomHex(16)
	oauthStates.Store(state, user.ID)

	// ClickUp's consent URL is simpler — no scopes needed
	consentURL := fmt.Sprintf(
		"https://app.clickup.com/api?client_id=%s&redirect_uri=%s&state=%s",
		clickupClientID,
		url.QueryEscape(clickupRedirectURI),
		state,
	)

	log.Printf("[ClickUp] Redirecting user %s to ClickUp consent", user.ID[:8])
	http.Redirect(w, r, consentURL, http.StatusTemporaryRedirect)
}

// STEP 2: ClickUp redirects back with an authorization code.
// We exchange it for an access token via a POST to ClickUp's token endpoint.
func handleClickUpCB(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	userIDVal, ok := oauthStates.LoadAndDelete(state)
	if !ok {
		http.Redirect(w, r, frontendURL+"?error=invalid_state", http.StatusTemporaryRedirect)
		return
	}
	userID := userIDVal.(string)

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		log.Printf("[ClickUp] Error: %s", errParam)
		http.Redirect(w, r, frontendURL+"?error="+errParam, http.StatusTemporaryRedirect)
		return
	}

	code := r.URL.Query().Get("code")

	// EXCHANGE: code → access_token
	// Unlike Google which uses form-encoded, ClickUp uses a JSON POST:
	//   POST https://api.clickup.com/api/v2/oauth/token
	//   Body: { "client_id": "...", "client_secret": "...", "code": "..." }
	reqBody, _ := json.Marshal(map[string]string{
		"client_id":     clickupClientID,
		"client_secret": clickupClientSecret,
		"code":          code,
	})

	resp, err := http.Post(
		"https://api.clickup.com/api/v2/oauth/token",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		log.Printf("[ClickUp] Token exchange failed: %v", err)
		http.Redirect(w, r, frontendURL+"?error=token_exchange_failed", http.StatusTemporaryRedirect)
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
		log.Printf("[ClickUp] Invalid token response")
		http.Redirect(w, r, frontendURL+"?error=invalid_token_response", http.StatusTemporaryRedirect)
		return
	}

	// Store as oauth2.Token for consistency with our token store
	token := &oauth2.Token{
		AccessToken: tokenResp.AccessToken,
		TokenType:   "Bearer",
	}
	tokenStore.Store(userID+":clickup", token)

	log.Printf("[ClickUp] SUCCESS — token stored for user %s", userID[:8])
	log.Printf("[ClickUp]   access_token: %s...", token.AccessToken[:15])

	http.Redirect(w, r, frontendURL+"?connected=clickup", http.StatusTemporaryRedirect)
}

func handleClickUpStatus(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	_, connected := tokenStore.Load(user.ID + ":clickup")
	writeJSON(w, http.StatusOK, map[string]any{
		"connected": connected,
		"provider":  "clickup",
	})
}

func handleClickUpDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	tokenStore.Delete(user.ID + ":clickup")
	log.Printf("[ClickUp] Disconnected for user %s", user.ID[:8])
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// =============================================================================
// CLICKUP DATA HANDLERS
//
// ClickUp API uses a simple Authorization header (no "Bearer" prefix needed,
// but it works with it too). The API is REST-based, similar to Google.
// =============================================================================

func handleClickUpProfile(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	data, err := callClickUpAPI(user.ID, "https://api.clickup.com/api/v2/user")
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"_meta": map[string]any{
			"api_url":     "https://api.clickup.com/api/v2/user",
			"http_method": "GET",
			"description": "Fetches the authenticated ClickUp user's profile",
		},
	})
}

// Fetch all workspaces (teams) the user belongs to.
func handleClickUpWorkspaces(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	data, err := callClickUpAPI(user.ID, "https://api.clickup.com/api/v2/team")
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"_meta": map[string]any{
			"api_url":     "https://api.clickup.com/api/v2/team",
			"http_method": "GET",
			"description": "Lists all ClickUp workspaces (teams) the user belongs to",
		},
	})
}

// Fetch tasks from the user's first workspace.
func handleClickUpTasks(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		httpError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	// First get the teams to find a team ID
	teamsData, err := callClickUpAPI(user.ID, "https://api.clickup.com/api/v2/team")
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}

	teams, ok := teamsData["teams"].([]any)
	if !ok || len(teams) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"tasks": []any{}},
			"_meta": map[string]any{
				"description": "No workspaces found — cannot fetch tasks",
			},
		})
		return
	}

	firstTeam := teams[0].(map[string]any)
	teamID := fmt.Sprintf("%v", firstTeam["id"])

	// Fetch tasks assigned to the authenticated user
	apiURL := fmt.Sprintf(
		"https://api.clickup.com/api/v2/team/%s/task?order_by=updated&reverse=true&subtasks=true&include_closed=false&page=0",
		teamID,
	)

	data, err := callClickUpAPI(user.ID, apiURL)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"_meta": map[string]any{
			"api_url":     apiURL,
			"http_method": "GET",
			"description": fmt.Sprintf("Fetches recent tasks from workspace '%s'", firstTeam["name"]),
		},
	})
}

// callClickUpAPI makes an authenticated GET request to the ClickUp API.
func callClickUpAPI(userID, apiURL string) (map[string]any, error) {
	token := getToken(userID, "clickup")
	if token == nil {
		return nil, fmt.Errorf("clickup not connected")
	}

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[ClickUp API] GET %s", apiURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[ClickUp API] returned %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("ClickUp API returned %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	json.Unmarshal(body, &result)
	return result, nil
}

// =============================================================================
// HELPERS
// =============================================================================

func currentUser(r *http.Request) *User {
	c, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	if val, ok := sessions.Load(c.Value); ok {
		return val.(*User)
	}
	return nil
}

func getToken(userID, provider string) *oauth2.Token {
	if val, ok := tokenStore.Load(userID + ":" + provider); ok {
		return val.(*oauth2.Token)
	}
	return nil
}

// googleHTTPClient returns an HTTP client that automatically injects the
// OAuth Bearer token into every request, and refreshes it when expired.
func googleHTTPClient(userID string) *http.Client {
	token := getToken(userID, "google")
	if token == nil {
		return nil
	}
	// oauth2.TokenSource handles refresh transparently:
	//   - If access_token is valid → use it
	//   - If expired → POST to Google with refresh_token → get new access_token
	ts := googleOAuth.TokenSource(context.Background(), token)

	// Persist the potentially-refreshed token
	if newToken, err := ts.Token(); err == nil && newToken.AccessToken != token.AccessToken {
		tokenStore.Store(userID+":google", newToken)
		log.Printf("[Token] Auto-refreshed access token for user %s", userID[:8])
	}

	return oauth2.NewClient(context.Background(), ts)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func httpError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

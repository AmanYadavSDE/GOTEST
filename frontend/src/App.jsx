import { useState, useEffect, useCallback } from 'react'

const API = import.meta.env.VITE_API_URL || 'http://localhost:3001'

function api(path, opts = {}) {
  return fetch(API + path, { credentials: 'include', ...opts })
    .then(async (r) => {
      const data = await r.json()
      if (!r.ok) throw new Error(data.error || 'Request failed')
      return data
    })
}

export default function App() {
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api('/api/me')
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('connected') || params.get('error')) {
      window.history.replaceState({}, '', '/')
    }
  }, [])

  if (loading) return <div className="loading-screen">Loading...</div>

  return user ? (
    <Dashboard user={user} onLogout={() => setUser(null)} />
  ) : (
    <LoginPage onLogin={setUser} />
  )
}

// =============================================================================
// LOGIN PAGE
// =============================================================================

function LoginPage({ onLogin }) {
  const [name, setName] = useState('')
  const [error, setError] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!name.trim()) return
    try {
      const user = await api('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() }),
      })
      onLogin(user)
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <div className="login-container">
      <div className="login-card">
        <div className="login-header">
          <div className="logo">WI</div>
          <h1>Workspace Insights</h1>
          <p>Connect your tools. Get actionable insights.</p>
        </div>
        <form onSubmit={handleSubmit}>
          <input
            type="text"
            placeholder="Enter your name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
          <button type="submit" className="btn-primary">Login</button>
          {error && <p className="error">{error}</p>}
        </form>
        <p className="hint">Demo auth — no password needed</p>
      </div>
    </div>
  )
}

// =============================================================================
// DASHBOARD
// =============================================================================

function Dashboard({ user, onLogout }) {
  const [googleStatus, setGoogleStatus] = useState(null)
  const [clickupStatus, setClickupStatus] = useState(null)
  const [error, setError] = useState('')

  const checkStatuses = useCallback(async () => {
    const [g, c] = await Promise.all([
      api('/api/google/status').catch(() => ({ connected: false })),
      api('/api/clickup/status').catch(() => ({ connected: false })),
    ])
    setGoogleStatus(g)
    setClickupStatus(c)
  }, [])

  useEffect(() => { checkStatuses() }, [checkStatuses])

  const logout = async () => {
    await api('/api/logout', { method: 'POST' }).catch(() => {})
    onLogout()
  }

  return (
    <div className="dashboard">
      <header className="topbar">
        <div className="topbar-left">
          <div className="logo-sm">WI</div>
          <h2>Workspace Insights</h2>
        </div>
        <div className="topbar-right">
          <span className="user-name">{user.name}</span>
          <button className="btn-ghost" onClick={logout}>Logout</button>
        </div>
      </header>

      <main className="main">
        {error && (
          <div className="alert alert-error">
            {error}
            <button onClick={() => setError('')}>&times;</button>
          </div>
        )}

        {/* PROVIDER CARDS */}
        <section className="section">
          <h3 className="section-title">Connected Providers</h3>
          <div className="providers-grid">
            <GoogleProviderCard
              status={googleStatus}
              onStatusChange={setGoogleStatus}
              onError={setError}
            />
            <ClickUpProviderCard
              status={clickupStatus}
              onStatusChange={setClickupStatus}
              onError={setError}
            />
          </div>
        </section>

        {/* GOOGLE DATA */}
        {googleStatus?.connected && (
          <GoogleDataSection onError={setError} />
        )}

        {/* CLICKUP DATA */}
        {clickupStatus?.connected && (
          <ClickUpDataSection onError={setError} />
        )}

        {/* HOW IT WORKS — when nothing is connected */}
        {!googleStatus?.connected && !clickupStatus?.connected && (
          <HowItWorks />
        )}
      </main>
    </div>
  )
}

// =============================================================================
// PROVIDER CARDS
// =============================================================================

function GoogleProviderCard({ status, onStatusChange, onError }) {
  const connected = status?.connected

  const disconnect = async () => {
    try {
      await api('/api/google/disconnect', { method: 'POST' })
      onStatusChange({ connected: false })
    } catch (err) { onError(err.message) }
  }

  return (
    <div className={`provider-card ${connected ? 'connected' : ''}`}>
      <div className="provider-info">
        <div className="provider-icon google-icon">G</div>
        <div>
          <h4>Google Workspace</h4>
          <p className="provider-desc">
            {connected
              ? 'Profile, Drive, Calendar, Gmail'
              : 'Connect to fetch workspace data'}
          </p>
          {connected && status.expires_at && (
            <p className="token-info">
              Token expires: {new Date(status.expires_at).toLocaleString()}
              {status.has_refresh_token && ' (auto-refreshes)'}
            </p>
          )}
        </div>
      </div>
      <div className="provider-actions">
        {connected ? (
          <button className="btn-danger" onClick={disconnect}>Disconnect</button>
        ) : (
          <a href={API + '/auth/google'} className="btn-google">
            <GoogleIcon /> Connect Google
          </a>
        )}
      </div>
    </div>
  )
}

function ClickUpProviderCard({ status, onStatusChange, onError }) {
  const connected = status?.connected

  const disconnect = async () => {
    try {
      await api('/api/clickup/disconnect', { method: 'POST' })
      onStatusChange({ connected: false })
    } catch (err) { onError(err.message) }
  }

  return (
    <div className={`provider-card ${connected ? 'connected' : ''}`}>
      <div className="provider-info">
        <div className="provider-icon clickup-icon">
          <ClickUpIcon />
        </div>
        <div>
          <h4>ClickUp</h4>
          <p className="provider-desc">
            {connected
              ? 'Profile, Workspaces, Tasks'
              : 'Connect to fetch project data'}
          </p>
          {connected && (
            <p className="token-info">Token: long-lived (no expiry)</p>
          )}
        </div>
      </div>
      <div className="provider-actions">
        {connected ? (
          <button className="btn-danger" onClick={disconnect}>Disconnect</button>
        ) : (
          <a href={API + '/auth/clickup'} className="btn-clickup">
            <ClickUpIcon /> Connect ClickUp
          </a>
        )}
      </div>
    </div>
  )
}

// =============================================================================
// GOOGLE DATA SECTION
// =============================================================================

function GoogleDataSection({ onError }) {
  const [profile, setProfile] = useState(null)
  const [drive, setDrive] = useState(null)
  const [calendar, setCalendar] = useState(null)
  const [gmail, setGmail] = useState(null)
  const [loadingStates, setLoadingStates] = useState({})
  const [activeTab, setActiveTab] = useState('profile')

  const setLoading = (key, val) =>
    setLoadingStates((prev) => ({ ...prev, [key]: val }))

  const fetchAll = () => {
    const fetchOne = async (key, path, setter) => {
      setLoading(key, true)
      try { setter(await api(path)) }
      catch (err) { setter({ error: err.message }) }
      finally { setLoading(key, false) }
    }
    fetchOne('profile', '/api/google/profile', setProfile)
    fetchOne('drive', '/api/google/drive', setDrive)
    fetchOne('calendar', '/api/google/calendar', setCalendar)
    fetchOne('gmail', '/api/google/gmail', setGmail)
  }

  useEffect(() => { fetchAll() }, [])

  const tabs = [
    { id: 'profile', label: 'Profile', icon: '👤' },
    { id: 'drive', label: 'Drive', icon: '📁' },
    { id: 'calendar', label: 'Calendar', icon: '📅' },
    { id: 'gmail', label: 'Gmail', icon: '✉️' },
  ]

  return (
    <section className="section">
      <div className="section-header">
        <h3 className="section-title">Google Data</h3>
        <button className="btn-secondary btn-sm" onClick={fetchAll}>Refresh All</button>
      </div>
      <div className="tabs">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={`tab ${activeTab === tab.id ? 'active' : ''}`}
            onClick={() => setActiveTab(tab.id)}
          >
            <span className="tab-icon">{tab.icon}</span>{tab.label}
          </button>
        ))}
      </div>
      <div className="tab-content">
        {activeTab === 'profile' && <ProfilePanel data={profile} loading={loadingStates.profile} />}
        {activeTab === 'drive' && <DrivePanel data={drive} loading={loadingStates.drive} />}
        {activeTab === 'calendar' && <CalendarPanel data={calendar} loading={loadingStates.calendar} />}
        {activeTab === 'gmail' && <GmailPanel data={gmail} loading={loadingStates.gmail} />}
      </div>
    </section>
  )
}

// =============================================================================
// CLICKUP DATA SECTION
// =============================================================================

function ClickUpDataSection({ onError }) {
  const [profile, setProfile] = useState(null)
  const [workspaces, setWorkspaces] = useState(null)
  const [tasks, setTasks] = useState(null)
  const [loadingStates, setLoadingStates] = useState({})
  const [activeTab, setActiveTab] = useState('profile')

  const setLoading = (key, val) =>
    setLoadingStates((prev) => ({ ...prev, [key]: val }))

  const fetchAll = () => {
    const fetchOne = async (key, path, setter) => {
      setLoading(key, true)
      try { setter(await api(path)) }
      catch (err) { setter({ error: err.message }) }
      finally { setLoading(key, false) }
    }
    fetchOne('profile', '/api/clickup/profile', setProfile)
    fetchOne('workspaces', '/api/clickup/workspaces', setWorkspaces)
    fetchOne('tasks', '/api/clickup/tasks', setTasks)
  }

  useEffect(() => { fetchAll() }, [])

  const tabs = [
    { id: 'profile', label: 'Profile', icon: '👤' },
    { id: 'workspaces', label: 'Workspaces', icon: '🏢' },
    { id: 'tasks', label: 'Tasks', icon: '✅' },
  ]

  return (
    <section className="section">
      <div className="section-header">
        <h3 className="section-title">ClickUp Data</h3>
        <button className="btn-secondary btn-sm" onClick={fetchAll}>Refresh All</button>
      </div>
      <div className="tabs tabs-clickup">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={`tab ${activeTab === tab.id ? 'active' : ''}`}
            onClick={() => setActiveTab(tab.id)}
          >
            <span className="tab-icon">{tab.icon}</span>{tab.label}
          </button>
        ))}
      </div>
      <div className="tab-content">
        {activeTab === 'profile' && <ClickUpProfilePanel data={profile} loading={loadingStates.profile} />}
        {activeTab === 'workspaces' && <ClickUpWorkspacesPanel data={workspaces} loading={loadingStates.workspaces} />}
        {activeTab === 'tasks' && <ClickUpTasksPanel data={tasks} loading={loadingStates.tasks} />}
      </div>
    </section>
  )
}

// =============================================================================
// GOOGLE DATA PANELS
// =============================================================================

function ProfilePanel({ data, loading }) {
  if (loading) return <LoadingState />
  if (!data?.data) return <EmptyState text="No profile data yet" />
  const p = data.data
  return (
    <div className="panel-grid">
      <div className="insight-card">
        <h4>Connected Account</h4>
        <div className="profile-row">
          {p.picture && <img src={p.picture} alt="" className="avatar" />}
          <div>
            <p className="profile-name">{p.name}</p>
            <p className="profile-email">{p.email}</p>
            {p.hd ? (
              <span className="badge">Workspace: {p.hd}</span>
            ) : (
              <span className="badge badge-grey">Personal Gmail</span>
            )}
          </div>
        </div>
      </div>
      <MetaCard meta={data._meta} />
    </div>
  )
}

function DrivePanel({ data, loading }) {
  if (loading) return <LoadingState />
  if (data?.error) return <ErrorState text={data.error} />
  if (!data?.data?.files) return <EmptyState text="No drive data yet" />
  const files = data.data.files
  return (
    <div className="panel-grid">
      <div className="insight-card full-width">
        <h4>Recent Files ({files.length})</h4>
        <p className="card-subtitle">Most recently modified files in Google Drive</p>
        <div className="data-table">
          <table>
            <thead><tr><th>Name</th><th>Type</th><th>Modified</th></tr></thead>
            <tbody>
              {files.map((f) => (
                <tr key={f.id}>
                  <td>
                    <span className="file-icon">{mimeIcon(f.mimeType)}</span>
                    {f.webViewLink ? <a href={f.webViewLink} target="_blank" rel="noreferrer">{f.name}</a> : f.name}
                  </td>
                  <td><code>{simplifyMime(f.mimeType)}</code></td>
                  <td>{f.modifiedTime ? new Date(f.modifiedTime).toLocaleDateString() : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <MetaCard meta={data._meta} />
    </div>
  )
}

function CalendarPanel({ data, loading }) {
  if (loading) return <LoadingState />
  if (data?.error) return <ErrorState text={data.error} />
  if (!data?.data?.items) return <EmptyState text="No calendar data yet" />
  const events = data.data.items
  return (
    <div className="panel-grid">
      <div className="insight-card full-width">
        <h4>Upcoming Events ({events.length})</h4>
        {events.length === 0 ? <p className="empty-text">No upcoming events</p> : (
          <div className="event-list">
            {events.map((e, i) => (
              <div className="event-item" key={i}>
                <div className="event-dot" />
                <div className="event-info">
                  <strong>{e.summary || '(No title)'}</strong>
                  <span className="event-time">{formatEventTime(e.start)}</span>
                  {e.attendees && <span className="event-attendees">{e.attendees.length} attendees</span>}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
      <MetaCard meta={data._meta} />
    </div>
  )
}

function GmailPanel({ data, loading }) {
  if (loading) return <LoadingState />
  if (data?.error) return <ErrorState text={data.error} />
  if (!data?.data) return <EmptyState text="No Gmail data yet" />
  const profile = data.data.profile
  const labels = data.data.labels?.labels || []
  const systemLabels = labels.filter((l) => l.type === 'system' && l.messagesTotal > 0)
  systemLabels.sort((a, b) => (b.messagesTotal || 0) - (a.messagesTotal || 0))
  return (
    <div className="panel-grid">
      {profile && (
        <div className="insight-card">
          <h4>Gmail Stats</h4>
          <div className="stat-grid">
            <div className="stat">
              <span className="stat-value">{(profile.messagesTotal || 0).toLocaleString()}</span>
              <span className="stat-label">Total Messages</span>
            </div>
            <div className="stat">
              <span className="stat-value">{(profile.threadsTotal || 0).toLocaleString()}</span>
              <span className="stat-label">Total Threads</span>
            </div>
          </div>
        </div>
      )}
      <div className="insight-card">
        <h4>Labels ({systemLabels.length})</h4>
        <div className="label-list">
          {systemLabels.slice(0, 10).map((l) => (
            <div className="label-item" key={l.id}>
              <span className="label-name">{l.name || l.id}</span>
              <div className="label-counts">
                <span>{(l.messagesTotal || 0).toLocaleString()} msgs</span>
                {l.messagesUnread > 0 && <span className="unread-badge">{l.messagesUnread} unread</span>}
              </div>
            </div>
          ))}
        </div>
      </div>
      <MetaCard meta={data._meta} />
    </div>
  )
}

// =============================================================================
// CLICKUP DATA PANELS
// =============================================================================

function ClickUpProfilePanel({ data, loading }) {
  if (loading) return <LoadingState />
  if (data?.error) return <ErrorState text={data.error} />
  if (!data?.data?.user) return <EmptyState text="No profile data yet" />
  const u = data.data.user
  return (
    <div className="panel-grid">
      <div className="insight-card">
        <h4>ClickUp Account</h4>
        <div className="profile-row">
          {u.profilePicture && <img src={u.profilePicture} alt="" className="avatar" />}
          <div>
            <p className="profile-name">{u.username}</p>
            <p className="profile-email">{u.email}</p>
            <span className="badge badge-purple">ID: {u.id}</span>
          </div>
        </div>
      </div>
      <MetaCard meta={data._meta} />
    </div>
  )
}

function ClickUpWorkspacesPanel({ data, loading }) {
  if (loading) return <LoadingState />
  if (data?.error) return <ErrorState text={data.error} />
  if (!data?.data?.teams) return <EmptyState text="No workspace data yet" />
  const teams = data.data.teams
  return (
    <div className="panel-grid">
      <div className="insight-card full-width">
        <h4>Workspaces ({teams.length})</h4>
        <p className="card-subtitle">ClickUp workspaces (teams) you belong to</p>
        <div className="workspace-list">
          {teams.map((t) => (
            <div className="workspace-item" key={t.id}>
              <div className="workspace-info">
                {t.avatar && <img src={t.avatar} alt="" className="workspace-avatar" />}
                <div>
                  <strong>{t.name}</strong>
                  <span className="workspace-meta">
                    {t.members?.length || 0} members
                  </span>
                </div>
              </div>
              <span className="badge badge-purple">ID: {t.id}</span>
            </div>
          ))}
        </div>
      </div>
      <MetaCard meta={data._meta} />
    </div>
  )
}

function ClickUpTasksPanel({ data, loading }) {
  if (loading) return <LoadingState />
  if (data?.error) return <ErrorState text={data.error} />
  if (!data?.data?.tasks) return <EmptyState text="No tasks data yet" />
  const tasks = data.data.tasks
  return (
    <div className="panel-grid">
      <div className="insight-card full-width">
        <h4>Recent Tasks ({tasks.length})</h4>
        <p className="card-subtitle">Most recently updated tasks</p>
        <div className="data-table">
          <table>
            <thead><tr><th>Task</th><th>Status</th><th>Priority</th><th>Assignees</th></tr></thead>
            <tbody>
              {tasks.slice(0, 15).map((t) => (
                <tr key={t.id}>
                  <td>
                    {t.url ? <a href={t.url} target="_blank" rel="noreferrer">{t.name}</a> : t.name}
                  </td>
                  <td>
                    <span
                      className="status-pill"
                      style={{ background: t.status?.color || '#ccc' }}
                    >
                      {t.status?.status || '—'}
                    </span>
                  </td>
                  <td>
                    {t.priority ? (
                      <span className="priority-pill" data-priority={t.priority?.id}>
                        {t.priority?.priority || '—'}
                      </span>
                    ) : '—'}
                  </td>
                  <td>
                    <div className="assignee-list">
                      {(t.assignees || []).map((a) => (
                        <span key={a.id} className="assignee-chip" title={a.username}>
                          {a.profilePicture
                            ? <img src={a.profilePicture} alt="" className="assignee-avatar" />
                            : a.initials || a.username?.[0] || '?'}
                        </span>
                      ))}
                      {(!t.assignees || t.assignees.length === 0) && '—'}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <MetaCard meta={data._meta} />
    </div>
  )
}

// =============================================================================
// SHARED COMPONENTS
// =============================================================================

function MetaCard({ meta }) {
  if (!meta) return null
  const urls = meta.google_api_urls || meta.api_urls || (meta.google_api_url ? [meta.google_api_url] : meta.api_url ? [meta.api_url] : [])
  return (
    <div className="insight-card full-width meta-card">
      <h4>Under The Hood</h4>
      <p className="card-subtitle">{meta.description || 'Your backend made this API call using the stored OAuth token:'}</p>
      <pre className="code-block">
{urls.map((u) => `${meta.http_method || 'GET'} ${u}`).join('\n')}
{urls.length > 0 ? `\nHeaders:\n  Authorization: Bearer ******* (stored token)` : ''}
{meta.scope_used ? `\nScope: ${meta.scope_used}` : ''}
      </pre>
    </div>
  )
}

function HowItWorks() {
  return (
    <section className="section">
      <h3 className="section-title">How It Works</h3>
      <div className="flow-steps">
        {[
          ['You click Connect', "Browser redirects to the provider's consent screen (Google, ClickUp, etc.)"],
          ['You grant permission', 'Provider sends an authorization code to your backend'],
          ['Backend exchanges code for tokens', 'Server-to-server call: code → access_token (+ refresh_token for Google)'],
          ['Backend calls provider APIs', 'Uses stored tokens to fetch your data — Drive, Calendar, Tasks, etc.'],
        ].map(([title, desc], i) => (
          <div className="step" key={i}>
            <div className="step-num">{i + 1}</div>
            <div>
              <strong>{title}</strong>
              <p>{desc}</p>
            </div>
          </div>
        ))}
      </div>
      <div className="comparison-box">
        <h4>Google vs ClickUp OAuth — Same Pattern, Different Details</h4>
        <table className="comparison-table">
          <thead><tr><th></th><th>Google</th><th>ClickUp</th></tr></thead>
          <tbody>
            <tr><td>Scopes</td><td>Granular (drive, calendar, gmail...)</td><td>No scopes — full access</td></tr>
            <tr><td>Token type</td><td>Short-lived + refresh token</td><td>Long-lived, no refresh</td></tr>
            <tr><td>Token exchange</td><td>Form-encoded POST</td><td>JSON POST</td></tr>
            <tr><td>Auth header</td><td>Bearer &lt;token&gt;</td><td>&lt;token&gt; (no Bearer prefix)</td></tr>
          </tbody>
        </table>
      </div>
    </section>
  )
}

function LoadingState() { return <div className="loading">Fetching data from API...</div> }
function EmptyState({ text }) { return <div className="empty-state">{text}</div> }

function ErrorState({ text }) {
  return (
    <div className="error-state">
      <strong>API Error</strong>
      <p>{text}</p>
      <p className="error-hint">Make sure the required API is enabled in your provider&apos;s console.</p>
    </div>
  )
}

function GoogleIcon() {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18">
      <path fill="#fff" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" />
      <path fill="#fff" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
      <path fill="#fff" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
      <path fill="#fff" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
    </svg>
  )
}

function ClickUpIcon() {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none">
      <path d="M4.5 16.5l3.5-2.7a5 5 0 0 0 8 0l3.5 2.7a9 9 0 0 1-15 0z" fill="currentColor" />
      <path d="M12 3l-8 10.5 3.5 2.7L12 10l4.5 6.2 3.5-2.7L12 3z" fill="currentColor" />
    </svg>
  )
}

// =============================================================================
// HELPERS
// =============================================================================

function mimeIcon(mime) {
  if (!mime) return '📄'
  if (mime.includes('folder')) return '📁'
  if (mime.includes('spreadsheet')) return '📊'
  if (mime.includes('presentation')) return '📽️'
  if (mime.includes('document') || mime.includes('word')) return '📝'
  if (mime.includes('pdf')) return '📕'
  if (mime.includes('image')) return '🖼️'
  if (mime.includes('video')) return '🎬'
  return '📄'
}

function simplifyMime(mime) {
  if (!mime) return 'unknown'
  return mime.replace('application/vnd.google-apps.', 'google/').replace('application/', '').replace('image/', 'img/')
}

function formatEventTime(start) {
  if (!start) return ''
  if (start.dateTime) return new Date(start.dateTime).toLocaleString(undefined, { weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
  if (start.date) return new Date(start.date).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' }) + ' (all day)'
  return ''
}

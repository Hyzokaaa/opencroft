import { useState } from 'react'
import Command from './Command.jsx'

export default function Login({ hasUsers, onSignedIn }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(null)
  const [busy, setBusy] = useState(false)

  async function submit(event) {
    event.preventDefault()
    setBusy(true)
    setError(null)

    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })

      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        setError(body.error ?? `The daemon answered ${res.status}`)
        return
      }
      onSignedIn()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-6">
      <div className="w-full max-w-sm">
        <h1 className="text-lg font-medium tracking-tight">OpenCroft</h1>

        {/* There is no sign-up: users are created on the host, by someone who
            already has a shell there. */}
        {!hasUsers ? (
          <div className="mt-4 rounded-lg border border-edge bg-panel px-4 py-4">
            <p className="text-sm">No users yet.</p>
            <p className="mt-1 text-xs text-muted">
              Create the first one on the server. There is no sign-up here on purpose.
            </p>
            <Command lines={['croft user add <name>']} className="mt-3" />
          </div>
        ) : (
          <form onSubmit={submit} className="mt-4 space-y-3">
            <Field
              label="Username"
              value={username}
              onChange={setUsername}
              autoFocus
              autoComplete="username"
            />
            <Field
              label="Password"
              type="password"
              value={password}
              onChange={setPassword}
              autoComplete="current-password"
            />

            {error && (
              <p className="rounded border border-problem/30 bg-problem/[0.06] px-3 py-2 text-xs text-problem">
                {error}
              </p>
            )}

            <button
              type="submit"
              disabled={busy || !username || !password}
              className="w-full rounded border border-edge-strong bg-raised px-3 py-2 text-sm transition hover:border-ink/30 disabled:opacity-40"
            >
              {busy ? 'Signing in…' : 'Sign in'}
            </button>
          </form>
        )}

        <p className="mt-6 text-xs text-faint">
          Forgot the password? Reset it on the host: <span className="font-mono">croft user rm</span>{' '}
          then <span className="font-mono">croft user add</span>. Nothing here can be recovered by
          email, because there is no email.
        </p>
      </div>
    </div>
  )
}

function Field({ label, value, onChange, type = 'text', ...rest }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs text-muted">{label}</span>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full rounded border border-edge bg-ground px-3 py-2 text-sm outline-none transition focus:border-edge-strong"
        {...rest}
      />
    </label>
  )
}

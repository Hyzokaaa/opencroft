// readJSON reads what the daemon answered without assuming it is JSON.
//
// `res.json()` on its own breaks in exactly the moments that matter: a proxy
// in front of croft answers a 502 with a page of HTML, a restart answers with
// nothing, and what reached the person was "Unexpected token '<'". An answer
// that is not ok is thrown with the daemon's own words when it sent any, and
// with words about the status when it did not. The status travels on the
// error, so a caller can tell "not here" from "not allowed" from "broken".
export async function readJSON(res) {
  const text = await res.text()
  let payload = null
  let parsed = !text
  if (text) {
    try {
      payload = JSON.parse(text)
      parsed = true
    } catch {
      // Said below, with what it means rather than what the parser saw.
    }
  }

  if (!res.ok) {
    const error = new Error(payload?.error ?? answered(res.status))
    error.status = res.status
    throw error
  }
  if (!parsed) {
    const error = new Error(
      'The answer was not something croft sends. Something between the panel and croft — a proxy, most often — answered instead.',
    )
    error.status = res.status
    throw error
  }
  return payload
}

// humane turns what went wrong into a sentence for the person in front of the
// panel. A failed fetch says "Failed to fetch" in one browser, "Load failed" in
// another and "NetworkError…" in a third; all three mean the same thing.
export function humane(e) {
  if (e?.name === 'TypeError' && /fetch|network|load failed/i.test(e.message ?? '')) {
    return 'The panel cannot reach croft. Check that the daemon is running and the connection is up, then try again.'
  }
  return e?.message || 'Something went wrong, and nothing said what.'
}

function answered(status) {
  switch (status) {
    case 401:
      return 'Your session has ended. Sign in again.'
    case 403:
      return 'This is not allowed on this panel.'
    case 404:
      return 'Croft does not know this any more — it may have been removed meanwhile.'
    case 502:
    case 503:
    case 504:
      return `Croft did not answer (${status}). It may be restarting; try again in a moment.`
    default:
      return `Croft answered ${status} and did not say why.`
  }
}

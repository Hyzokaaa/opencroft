// A site has no process, so whether its web server is up says little: what
// counts is what a visitor gets. The agent asks for the front page each time
// the container is read, and this is that answer in a word.
//
// state is the same vocabulary as a unit's, so the dots and pills that
// already exist can show it: green only for a page that came back.
export function servingOf(service) {
  const s = service.serving
  if (!s) {
    return {
      label: 'unknown',
      state: 'unknown',
      sentence: 'has not been asked for yet, so whether it serves is unknown.',
    }
  }
  if (s.problem) {
    return { label: 'no answer', state: 'failed', sentence: `gets no answer: ${s.problem}.` }
  }
  if (s.status < 300) {
    return { label: 'serving', state: 'active', sentence: `answers ${s.status} at ${s.domain}.` }
  }
  if (s.status < 400) {
    return {
      label: `redirects · ${s.status}`,
      state: 'unknown',
      sentence: `answers ${s.status} at ${s.domain}, sending visitors to another address.`,
    }
  }
  // A 4xx at the root is the web server answering, often on purpose — a site
  // that lives under a path, or asks to sign in first. It is worth a look,
  // not an alarm.
  if (s.status < 500) {
    return {
      label: `serving · HTTP ${s.status}`,
      state: 'unknown',
      caution: true,
      sentence: `answers ${s.status} at the front page of ${s.domain}. If that is not on purpose, visitors see it too.`,
    }
  }
  return {
    label: `not serving · HTTP ${s.status}`,
    state: 'failed',
    sentence: `answers ${s.status} at ${s.domain}: visitors get an error page.`,
  }
}

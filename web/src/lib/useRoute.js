import { useCallback, useEffect, useState } from 'react'

// Where you are is the address, so a reload keeps you there, Back goes back,
// and a container can be linked to. A hash rather than a path because the
// daemon serves one page and should not have to know the panel's routes.
//
//   #/                       home: the projects and what needs attention
//   #/projects/:name         one project
//   #/containers?status=…    every container, optionally filtered
//   #/containers/:name       one container
//   #/domains                domains
//   #/domains/certificates   certificates
//   #/activity, #/settings
export function parse(hash) {
  const [path, search = ''] = (hash || '').replace(/^#/, '').split('?')
  const parts = path.split('/').filter(Boolean).map(decodeURIComponent)
  const query = Object.fromEntries(new URLSearchParams(search))

  const [section = 'home', name = null] = parts
  if (section === 'domains') {
    return { section, tab: name === 'certificates' ? 'certificates' : 'domains', name: null, query }
  }
  return { section, name, tab: null, query }
}

export function href(section, name, query) {
  let out = '#/' + (section === 'home' ? '' : section)
  if (name) out += '/' + encodeURIComponent(name)
  const search = new URLSearchParams(Object.entries(query ?? {}).filter(([, v]) => v)).toString()
  return search ? `${out}?${search}` : out
}

export function useRoute() {
  const [route, setRoute] = useState(() => parse(location.hash))

  useEffect(() => {
    const read = () => {
      setRoute(parse(location.hash))
      window.scrollTo(0, 0)
    }
    addEventListener('hashchange', read)
    return () => removeEventListener('hashchange', read)
  }, [])

  const navigate = useCallback((section, name, query) => {
    location.hash = href(section, name, query)
  }, [])

  return [route, navigate]
}

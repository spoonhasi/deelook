// The page's address as state: the app picks the page by the path, the HMI
// its machine by ?machine=. Both change it through go(), so the address,
// the back button and what is on screen agree.
class Address {
  path = $state('')
  search = $state('')

  constructor() {
    // addresses of earlier builds, #/ (the HMI) and #/sampling, still lead there
    if (location.hash.startsWith('#/')) {
      const to = location.hash.slice(1)
      history.replaceState(null, '', (to === '/' ? '/hmi' : to) + location.search)
    }
    this.path = location.pathname
    this.search = location.search
    addEventListener('popstate', () => {
      this.path = location.pathname
      this.search = location.search
    })
  }

  /** The machine named by ?machine=, '' for the server's default. */
  get machine(): string {
    return new URLSearchParams(this.search).get('machine') ?? ''
  }

  /** Goes to path + search, as a new entry in the browser's history. */
  go(path: string, search = this.search) {
    if (path === location.pathname && search === location.search) return
    history.pushState(null, '', path + search)
    this.path = path
    this.search = search
  }

  /** The current search with ?machine= set to machine. */
  withMachine(machine: string): string {
    const q = new URLSearchParams(this.search)
    q.set('machine', machine)
    return '?' + q
  }
}

export const address = new Address()

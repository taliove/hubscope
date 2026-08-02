// Sidebar footer version shortener (GH #146, spec 0019 T6). The deploy.sh
// build stamp is `dev-<git describe --tags --always --dirty>-YYYYMMDD-HHMMSS`
// (36 chars) which overflows the single-line legal row; the display layer
// shortens dev stamps to `dev-g<hash>` while the title attribute keeps the
// full stamp. Release tags keep the historical ^v\d+\.\d+\.\d+ behavior.
export function shortVersion(full: string): string {
  if (!full) return ''
  if (!full.startsWith('dev-')) {
    const match = full.match(/^v\d+\.\d+\.\d+/)
    return match ? match[0] : full
  }
  // Drop the trailing date-time segments, then extract the hash segment.
  const body = full.slice(4).replace(/-\d{8}-\d{6}$/, '')
  const described = body.match(/-g([0-9a-f]+?)(?:-dirty)?$/)
  if (described) return `dev-g${described[1]}`
  const plainHash = body.match(/^([0-9a-f]{7,40})(?:-dirty)?$/)
  if (plainHash) return `dev-${plainHash[1]}`
  // No hash to extract (e.g. `nogit` fallback when git describe fails):
  // never invent one — show the describe body as-is.
  return `dev-${body}`
}

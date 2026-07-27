export interface VersionResponse {
  version: string
}

export async function fetchVersion(): Promise<VersionResponse> {
  const res = await fetch('/api/version')
  if (!res.ok) {
    throw new Error(`Failed to fetch version: ${res.statusText}`)
  }
  const json = await res.json()
  return json.data
}

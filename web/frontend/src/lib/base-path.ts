/**
 * Derives the runtime base path from Vite's BASE_URL which is injected at
 * build time from the `base` option in vite.config.ts (or `VITE_BASE_URL` env var).
 *
 * Always normalized to start and end with "/".
 * Defaults to "/" so all existing root-path deployments are unaffected.
 */
export const BASE_PATH: string = (() => {
  const raw = import.meta.env.BASE_URL || "/"
  // Ensure it starts with "/"
  let normalized = raw.startsWith("/") ? raw : `/${raw}`
  // Ensure it ends with "/"
  if (!normalized.endsWith("/")) {
    normalized += "/"
  }
  return normalized
})()

/**
 * Prepends the runtime base path to an absolute API/page path.
 *
 * Examples (with BASE_PATH = "/picoclaw/"):
 *   withBase("/api/models")      → "/picoclaw/api/models"
 *   withBase("/launcher-login")  → "/picoclaw/launcher-login"
 *
 * When BASE_PATH is "/" the input is returned unchanged, so root-path
 * deployments keep working without any config changes.
 */
export function withBase(path: string): string {
  if (!path.startsWith("/")) {
    path = `/${path}`
  }
  // Avoid double slashes when base is "/"
  if (BASE_PATH === "/") return path
  return `${BASE_PATH}${path.slice(1)}`
}

/**
 * Builds the WebSocket base URL including the runtime base path.
 *
 * Example (with BASE_PATH = "/picoclaw/"):
 *   getWsBaseUrl()  → "ws://example.com/picoclaw/"
 *
 * Callers should append the remaining path segment, e.g.:
 *   `${getWsBaseUrl()}pico/ws`
 */
export function getWsBaseUrl(): string {
  const wsScheme = window.location.protocol === "https:" ? "wss:" : "ws:"
  return `${wsScheme}//${window.location.host}${BASE_PATH}`
}

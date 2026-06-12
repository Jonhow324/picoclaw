import { BASE_PATH } from "@/lib/base-path"

/** Normalize URL pathname for comparisons (trailing slashes, empty). */
export function normalizePathname(p: string): string {
  const t = p.replace(/\/+$/, "")
  return t === "" ? "/" : t
}

/**
 * Strips the runtime base path prefix from a pathname so that comparisons
 * work regardless of whether the app is deployed at "/" or "/picoclaw/".
 *
 * Example (BASE_PATH = "/picoclaw/"):
 *   stripBase("/picoclaw/launcher-login")  → "/launcher-login"
 *   stripBase("/launcher-login")            → "/launcher-login"  (no-op when no prefix)
 */
function stripBase(pathname: string): string {
  if (BASE_PATH === "/") return pathname
  // Remove trailing slash from base for prefix matching
  const base = BASE_PATH.replace(/\/$/, "")
  if (pathname.startsWith(base)) {
    const remainder = pathname.slice(base.length)
    return remainder === "" ? "/" : remainder
  }
  return pathname
}

export function isLauncherLoginPathname(pathname: string): boolean {
  return normalizePathname(stripBase(pathname)) === "/launcher-login"
}

export function isLauncherSetupPathname(pathname: string): boolean {
  return normalizePathname(stripBase(pathname)) === "/launcher-setup"
}

/** True for any page that is part of the auth flow (login or setup). */
export function isLauncherAuthPathname(pathname: string): boolean {
  return isLauncherLoginPathname(pathname) || isLauncherSetupPathname(pathname)
}

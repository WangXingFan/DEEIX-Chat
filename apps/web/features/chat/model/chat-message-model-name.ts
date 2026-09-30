export function resolveMessageModelName(
  platformModelName?: string,
  upstreamModelName?: string,
  displayName?: string,
): string {
  const name = platformModelName?.trim() || "";
  if (displayName?.trim()) return displayName.trim();
  if (/^user_\d+_\d+_[0-9a-f]{32}$/.test(name)) {
    return upstreamModelName?.trim() || name;
  }
  return name;
}

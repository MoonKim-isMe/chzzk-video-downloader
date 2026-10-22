// Only the auto-open modal is postponed; explicit checks still display the update.
const postponedVersionKey = 'chzzk-video-downloader.postponed-update-version';

export function getPostponedUpdateVersion(): string | null {
  try {
    return window.localStorage.getItem(postponedVersionKey);
  } catch {
    // An unavailable storage service must not prevent checking for updates.
    return null;
  }
}

export function postponeUpdateVersion(version: string): void {
  if (!version.trim()) {
    return;
  }
  try {
    window.localStorage.setItem(postponedVersionKey, version);
  } catch {
    // If persistence is unavailable, the current modal can still be closed.
  }
}

export function shouldOpenUpdateModal(
  latestVersion: string,
  postponedVersion: string | null,
  manuallyRequested: boolean,
): boolean {
  return manuallyRequested || latestVersion !== postponedVersion;
}

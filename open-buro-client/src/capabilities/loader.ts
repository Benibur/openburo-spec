import { OBCError } from '../errors.js';
import type { Capability } from '../types.js';

export interface LoaderResult {
  capabilities: Capability[];
  isOpenBuroServer: boolean;
}

/**
 * Fetches capabilities from the given URL.
 *
 * CAP-05: Rejects non-HTTPS URLs when the host page is served over HTTPS.
 * CAP-04: Passes AbortSignal to fetch for lifecycle-safe teardown.
 * CAP-03: Rejects on non-2xx responses with OBCError(CAPABILITIES_FETCH_FAILED).
 * CAP-02: Reads X-OpenBuro-Server header into isOpenBuroServer.
 * CAP-01: Parses response JSON as Capability[].
 */
export async function fetchCapabilities(url: string, signal?: AbortSignal): Promise<LoaderResult> {
  // CAP-05: reject non-HTTPS at runtime when the host page itself is HTTPS
  if (
    new URL(url).protocol !== 'https:' &&
    typeof location !== 'undefined' &&
    location.protocol === 'https:'
  ) {
    throw new OBCError(
      'CAPABILITIES_FETCH_FAILED',
      `capabilitiesUrl must use HTTPS when host page is HTTPS: ${url}`,
    );
  }

  let response: Response;
  try {
    response = await fetch(url, { signal });
  } catch (cause) {
    throw new OBCError(
      'CAPABILITIES_FETCH_FAILED',
      `Failed to fetch capabilities from ${url}`,
      cause,
    );
  }

  if (!response.ok) {
    throw new OBCError(
      'CAPABILITIES_FETCH_FAILED',
      `Capabilities endpoint returned ${response.status}`,
    );
  }

  const isOpenBuroServer = response.headers.get('X-OpenBuro-Server') === 'true';
  const capabilities = (await response.json()) as Capability[];
  return { capabilities, isOpenBuroServer };
}

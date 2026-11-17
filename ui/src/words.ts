// Small helpers that turn what the server holds into what a person reads.
import type { ModuleHost } from '@prusa/connect-ui/slots';

import type { Notice } from './api';

type Translate = ModuleHost['t'];

/** How long the pet keeps repeating its comment on an event. */
const QUIP_FRESH_MS = 10 * 60 * 1000;

/** The pet's newest comment, while it is still news. */
export const latestQuip = (
  notices: readonly Notice[],
  now = Date.now(),
): string | undefined => {
  const newest = notices.find((notice) => notice.quip);
  if (!newest?.quip) return undefined;
  return now - Date.parse(newest.at) < QUIP_FRESH_MS ? newest.quip : undefined;
};

/** One notice as a sentence, in the language on screen. */
export const noticeText = (t: Translate, notice: Notice): string =>
  t(`print-ai-gotchi.kind.${notice.kind}`, {
    device: notice.device_name,
    detail: notice.detail ?? '',
  });

/** A byte count a person reads, such as 1.1 GB. */
export const sizeText = (bytes: number): string => {
  const format = (value: number, digits: number, unit: string): string =>
    `${value.toLocaleString(undefined, { maximumFractionDigits: digits })} ${unit}`;
  if (bytes >= 1e9) return format(bytes / 1e9, 1, 'GB');
  if (bytes >= 1e6) return format(bytes / 1e6, 0, 'MB');
  return format(bytes / 1e3, 0, 'kB');
};

/** The time of a notice, short. */
export const timeText = (at: string): string =>
  new Date(at).toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  });

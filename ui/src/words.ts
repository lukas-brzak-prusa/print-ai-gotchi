// Small helpers that turn what the server holds into what a person reads.
import type { ModuleHost } from '@prusa/connect-ui/slots';

import type { Notice, Roast } from './api';

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
export const noticeText = (t: Translate, notice: Notice): string => {
  const detail = notice.detail ?? '';
  return t(`print-ai-gotchi.kind.${notice.kind}`, {
    device: notice.device_name,
    // A pet that grew names its new stage, which the page translates.
    detail: detailText(t, notice.kind, detail),
  });
};

/** The detail of a notice about the pet: a stage to translate, a day to
 * format, or the machine's own words. */
const detailText = (t: Translate, kind: string, detail: string): string => {
  if (kind === 'evolved') return t(`print-ai-gotchi.stage.${detail}`);
  if (kind === 'roast' && /^\d{4}-\d{2}-\d{2}$/.test(detail)) {
    return new Date(`${detail}T12:00:00`).toLocaleDateString(undefined, {
      weekday: 'long',
      day: 'numeric',
      month: 'numeric',
    });
  }
  return detail;
};

/** The roast to show this morning: the newest, while it is under a day old. */
export const freshRoast = (
  roast: Roast | undefined,
  now = Date.now(),
): Roast | undefined =>
  roast && now - Date.parse(roast.at) < 24 * 60 * 60 * 1000 ? roast : undefined;

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

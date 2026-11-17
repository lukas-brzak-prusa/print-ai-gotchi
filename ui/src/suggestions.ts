// The questions the chat offers, from what the fleet is doing right now: a
// printer in trouble first, then one that prints, then the evergreens.
import type { ModuleHost } from '@prusa/connect-ui/slots';

type Device = ModuleHost['fleet']['devices'][number];

export interface Suggestion {
  key: string;
  params?: Record<string, string>;
}

const stateOf = (device: Device): string => {
  const state = (device.state as Record<string, unknown> | undefined)?.[
    '/state'
  ];
  return typeof state === 'string' ? state : '';
};

export const suggestionsFor = (devices: readonly Device[]): Suggestion[] => {
  const out: Suggestion[] = [];

  const troubled = devices.find(
    (device) =>
      (device.issues?.length ?? 0) > 0 ||
      ['ERROR', 'ATTENTION'].includes(stateOf(device)),
  );
  if (troubled) {
    out.push({
      key: 'trouble',
      params: { name: troubled.name },
    });
  }

  const printing = devices.find((device) => stateOf(device) === 'PRINTING');
  if (printing) {
    out.push({ key: 'eta', params: { name: printing.name } });
  }

  if (!troubled) out.push({ key: 'needs' });
  out.push({ key: 'status' }, { key: 'joke' });

  return out.slice(0, 4);
};

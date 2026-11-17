// The module's own routes, called through the client the host handed over, so
// every call carries the person's session and goes to the mount the host gave
// this module. The namespace is read from the host's own page address rather
// than written here, so a fork under another repository works unchanged.
import type { Operation, OperationRequest } from '@prusa/connect';
import type { ModuleHost } from '@prusa/connect-ui/slots';

export type Face =
  'alarmed' | 'worried' | 'busy' | 'proud' | 'hungry' | 'sleepy' | 'bored';
export type Stage = 'baby' | 'kid' | 'adult' | 'legend';
export type Hunger = 'full' | 'peckish' | 'hungry' | 'starving';
export type Species = 'cat' | 'robot' | 'blob';

export interface Profile {
  name: string;
  personality: string;
  language: string;
  species: Species;
}

export interface Mood {
  face: Face;
  printers: number;
  online: number;
  printing: number;
  problems: number;
}

export interface Notice {
  id: string;
  at: string;
  device_id: string;
  device_name: string;
  kind: string;
  severity: 'info' | 'warning' | 'error';
  detail?: string;
  quip?: string;
}

export interface Provider {
  id: string;
  title: string;
  kind: 'local' | 'openai' | 'anthropic';
  base_url: string;
  needs_key: boolean;
  on_premises: boolean;
  editable_url: boolean;
  key_url?: string;
}

export interface LocalModel {
  key: string;
  title: string;
  maker: string;
  params: number;
  multilingual: boolean;
  for: string;
  license: string;
  license_url: string;
  memory: number;
  measured: boolean;
  size: number;
}

export interface Download {
  model: string;
  step: 'runtime' | 'model' | '';
  done: number;
  total: number;
  error?: string;
  running: boolean;
}

export type LocalState =
  'absent' | 'starting' | 'ready' | 'failed' | 'off' | 'no_data';

export interface LocalStatus {
  state: LocalState;
  model: string;
  error?: string;
  installed: string[] | null;
  download?: Download;
  platform: string;
  supported: boolean;
  budget: number;
}

export interface ChoiceView {
  provider: string;
  local_model: string;
  base_url: Record<string, string>;
  model: Record<string, string>;
  has_key: Record<string, boolean>;
}

export interface Mind {
  choice: ChoiceView;
  providers: Provider[];
  models: LocalModel[];
  local: LocalStatus;
  ready: boolean;
  editable: boolean;
}

export interface ChoiceChange {
  provider: string;
  local_model?: string;
  base_url?: string;
  model?: string;
  key?: string;
}

export interface ProbeResult {
  ok: boolean;
  models: string[];
  error?: string;
}

/** The brain in one line, for the pet's header. */
export interface Summary {
  provider: string;
  title: string;
  model: string;
  on_premises: boolean;
  ready: boolean;
  state?: LocalState;
}

export interface Roast {
  date: string;
  text: string;
  at: string;
}

/** The pet's body. It counts the whole fleet and names no printer. */
export interface Vitals {
  stage: Stage;
  hunger: Hunger;
  /** 100 just after a print, 0 when starving begins. */
  fullness: number;
  hours: number;
  fed: string;
  born: string;
  next?: Stage;
  /** The share of the way to the next stage, 0 to 100. */
  growth: number;
  roast?: Roast;
}

export interface PetView {
  profile: Profile;
  mood: Mood;
  notices: Notice[];
  brain: Summary;
  choices: { personalities: string[]; languages: string[]; species: Species[] };
  vitals?: Vitals;
}

export interface Turn {
  role: 'user' | 'assistant';
  content: string;
}

export interface Reply {
  id: string;
  state: 'queued' | 'writing' | 'done' | 'failed';
  text: string;
  error?: string;
}

/** The namespace out of `/m/<namespace>/<path>`, which is what pageHref answers. */
export const namespaceOf = (host: ModuleHost): string => {
  const match = /\/m\/([^/]+)\//.exec(host.pageHref('print-ai-gotchi'));
  if (!match?.[1])
    throw new Error('the host gave print-ai-gotchi no page address');
  return match[1];
};

export interface Api {
  pet(signal?: AbortSignal): Promise<PetView>;
  saveProfile(profile: Profile, signal?: AbortSignal): Promise<Profile>;
  mind(signal?: AbortSignal): Promise<Mind>;
  choose(change: ChoiceChange, signal?: AbortSignal): Promise<Mind>;
  probe(
    probe: { provider: string; base_url?: string; key?: string },
    signal?: AbortSignal,
  ): Promise<ProbeResult>;
  install(model: string, signal?: AbortSignal): Promise<Mind>;
  remove(model: string, signal?: AbortSignal): Promise<Mind>;
  cancelDownload(signal?: AbortSignal): Promise<Mind>;
  ask(messages: Turn[], signal?: AbortSignal): Promise<Reply>;
  reply(id: string, signal?: AbortSignal): Promise<Reply>;
}

export const apiOf = (host: ModuleHost): Api => {
  const base = `/api/v1/modules/${namespaceOf(host)}`;
  const operation = (
    id: string,
    method: Operation['method'],
    path: string,
    body = false,
  ): Operation => ({
    id,
    method,
    path: base + path,
    response: 'json',
    anonymous: false,
    ...(body ? { body: 'application/json' } : {}),
  });
  const call = <T>(
    op: Operation,
    request: OperationRequest,
    signal?: AbortSignal,
  ): Promise<T> => host.client.transport.call<T>(op, request, signal);

  const pet = operation('print-ai-gotchi-pet', 'GET', '/pet');
  const profile = operation('print-ai-gotchi-profile', 'PUT', '/profile', true);
  const mind = operation('print-ai-gotchi-mind', 'GET', '/brain');
  const choose = operation('print-ai-gotchi-choose', 'PUT', '/brain', true);
  const probe = operation(
    'print-ai-gotchi-probe',
    'POST',
    '/brain/probe',
    true,
  );
  const install = operation(
    'print-ai-gotchi-install',
    'POST',
    '/brain/models/{model}/install',
  );
  const remove = operation(
    'print-ai-gotchi-remove',
    'DELETE',
    '/brain/models/{model}',
  );
  const cancelDownload = operation(
    'print-ai-gotchi-cancel-download',
    'DELETE',
    '/brain/download',
  );
  const ask = operation('print-ai-gotchi-ask', 'POST', '/chat', true);
  const reply = operation('print-ai-gotchi-reply', 'GET', '/chat/{id}');

  return {
    pet: (signal) => call(pet, {}, signal),
    saveProfile: (body, signal) => call(profile, { body }, signal),
    mind: (signal) => call(mind, {}, signal),
    choose: (body, signal) => call(choose, { body }, signal),
    probe: (body, signal) => call(probe, { body }, signal),
    install: (model, signal) => call(install, { path: { model } }, signal),
    remove: (model, signal) => call(remove, { path: { model } }, signal),
    cancelDownload: (signal) => call(cancelDownload, {}, signal),
    ask: (messages, signal) => call(ask, { body: { messages } }, signal),
    reply: (id, signal) => call(reply, { path: { id } }, signal),
  };
};

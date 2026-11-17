// The pet as the browser holds it: one store for the page and the card, and
// the conversation of the person at this browser. The conversation lives here
// and not on the server, so nobody else on the server reads it.
import type { ModuleHost } from '@prusa/connect-ui/slots';

import {
  type Api,
  apiOf,
  type ChoiceChange,
  type Mind,
  type PetView,
  type ProbeResult,
  type Profile,
  type Turn,
} from './api';

/** What one bubble in the chat shows. */
export interface Bubble extends Turn {
  pending?: boolean;
  failed?: boolean;
  at: number;
}

const POLL_PET_MS = 4000;
const POLL_REPLY_MS = 400;
const POLL_MIND_MS = 1500;

const errorText = (error: unknown): string =>
  error instanceof Error ? error.message : String(error);

const wait = (ms: number, signal: AbortSignal): Promise<void> =>
  new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      'abort',
      () => {
        clearTimeout(timer);
        reject(
          signal.reason instanceof Error ? signal.reason : new Error('aborted'),
        );
      },
      { once: true },
    );
  });

export class Pet {
  view = $state<PetView | undefined>();
  error = $state<string | undefined>();
  bubbles = $state<Bubble[]>([]);
  thinking = $state(false);
  saving = $state(false);

  readonly #api: Api;
  readonly #signal: AbortSignal;
  #watchers = 0;
  #timer: ReturnType<typeof setTimeout> | undefined;

  constructor(host: ModuleHost) {
    this.#api = apiOf(host);
    this.#signal = host.signal;
    host.signal.addEventListener(
      'abort',
      () => {
        clearTimeout(this.#timer);
      },
      { once: true },
    );
  }

  async load(): Promise<void> {
    try {
      this.view = await this.#api.pet(this.#signal);
      this.error = undefined;
    } catch (error) {
      if (!this.#signal.aborted) this.error = errorText(error);
    }
  }

  /**
   * Keeps the pet fresh while a page that shows it is open. The returned
   * function stops it, and the last one to stop ends the polling.
   */
  watch(): () => void {
    this.#watchers += 1;
    if (this.#watchers === 1) void this.#tick();
    return () => {
      this.#watchers -= 1;
      if (this.#watchers === 0) clearTimeout(this.#timer);
    };
  }

  async #tick(): Promise<void> {
    await this.load();
    if (this.#watchers > 0 && !this.#signal.aborted) {
      this.#timer = setTimeout(() => void this.#tick(), POLL_PET_MS);
    }
  }

  async saveProfile(profile: Profile): Promise<void> {
    this.saving = true;
    try {
      const saved = await this.#api.saveProfile(profile, this.#signal);
      if (this.view) this.view = { ...this.view, profile: saved };
      this.error = undefined;
    } catch (error) {
      this.error = errorText(error);
    } finally {
      this.saving = false;
    }
  }

  mind = $state<Mind | undefined>();
  mindBusy = $state(false);
  #mindTimer: ReturnType<typeof setTimeout> | undefined;

  async loadMind(): Promise<void> {
    try {
      this.mind = await this.#api.mind(this.#signal);
      this.error = undefined;
    } catch (error) {
      if (!this.#signal.aborted) this.error = errorText(error);
    }
  }

  /** Follows the brain while its tab is open: a download moves every second. */
  watchMind(): () => void {
    let live = true;
    const tick = async (): Promise<void> => {
      await this.loadMind();
      if (live && !this.#signal.aborted) {
        this.#mindTimer = setTimeout(() => void tick(), POLL_MIND_MS);
      }
    };
    void tick();
    return () => {
      live = false;
      clearTimeout(this.#mindTimer);
    };
  }

  async #mindAction(action: () => Promise<Mind>): Promise<boolean> {
    this.mindBusy = true;
    try {
      this.mind = await action();
      this.error = undefined;
      void this.load();
      return true;
    } catch (error) {
      this.error = errorText(error);
      return false;
    } finally {
      this.mindBusy = false;
    }
  }

  choose(change: ChoiceChange): Promise<boolean> {
    return this.#mindAction(() => this.#api.choose(change, this.#signal));
  }

  install(model: string): Promise<boolean> {
    return this.#mindAction(() => this.#api.install(model, this.#signal));
  }

  remove(model: string): Promise<boolean> {
    return this.#mindAction(() => this.#api.remove(model, this.#signal));
  }

  async probe(probe: {
    provider: string;
    base_url?: string;
    key?: string;
  }): Promise<ProbeResult> {
    try {
      return await this.#api.probe(probe, this.#signal);
    } catch (error) {
      return { ok: false, models: [], error: errorText(error) };
    }
  }

  async ask(question: string): Promise<void> {
    const text = question.trim();
    if (!text || this.thinking) return;

    const history: Turn[] = this.bubbles
      .filter((bubble) => !bubble.pending && !bubble.failed)
      .map(({ role, content }) => ({ role, content }));
    history.push({ role: 'user', content: text });

    this.bubbles.push({ role: 'user', content: text, at: Date.now() });
    this.bubbles.push({
      role: 'assistant',
      content: '',
      pending: true,
      at: Date.now(),
    });
    const answer = this.bubbles[this.bubbles.length - 1];
    if (!answer) return;
    this.thinking = true;

    try {
      let reply = await this.#api.ask(history, this.#signal);
      while (reply.state === 'queued' || reply.state === 'writing') {
        await wait(POLL_REPLY_MS, this.#signal);
        reply = await this.#api.reply(reply.id, this.#signal);
        answer.content = reply.text;
      }
      answer.content = reply.text;
      answer.pending = false;
      if (reply.state === 'failed') {
        answer.failed = true;
        answer.content = reply.error ?? '';
      }
    } catch (error) {
      answer.pending = false;
      answer.failed = true;
      answer.content = errorText(error);
    } finally {
      this.thinking = false;
    }
  }

  forget(): void {
    this.bubbles = [];
  }
}

let held: Pet | undefined;

/** The one pet of this browser, built in the module's setup. */
export const holdPet = (host: ModuleHost): void => {
  held = new Pet(host);
};

export const heldPet = (): Pet => {
  if (!held)
    throw new Error('the print-ai-gotchi module drew before its setup ran');
  return held;
};

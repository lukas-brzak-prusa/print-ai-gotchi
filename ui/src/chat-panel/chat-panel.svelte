<script lang="ts">
  // The conversation with the pet. It is kept in this browser only.
  import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
  import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
  import { Button, Chip, Textarea } from '@prusa/connect-ui/primitives';
  import { tick } from 'svelte';

  import type { PetView } from '../api';
  import { heldHost } from '../host';
  import { heldPet } from '../pet.svelte';
  import PetFace from '../pet-face/pet-face.svelte';
  import { suggestionsFor } from '../suggestions';
  import { freshRoast } from '../words';

  interface Props {
    view: PetView;
    /** Opens the brain tab, for a pet that has none yet. */
    onSetup: () => void;
  }

  let { view, onSetup }: Props = $props();

  const host = heldHost();
  const t = host.t;
  const pet = heldPet();
  const uid = $props.id();

  let question = $state('');
  let log: HTMLDivElement | undefined = $state();

  const awake = $derived(view.brain.ready);
  const name = $derived(view.profile.name);
  const suggestions = $derived(suggestionsFor(host.fleet.devices));
  const roast = $derived(freshRoast(view.vitals?.roast));

  const send = async (text: string): Promise<void> => {
    if (!text.trim() || pet.thinking) return;
    question = '';
    const asked = pet.ask(text);
    await tick();
    log?.scrollTo({ top: log.scrollHeight, behavior: 'smooth' });
    await asked;
  };

  // Follow the answer while the pet writes it.
  $effect(() => {
    const last = pet.bubbles[pet.bubbles.length - 1];
    if (last?.content !== undefined && log) {
      log.scrollTo({ top: log.scrollHeight });
    }
  });

  const onKey = (event: KeyboardEvent): void => {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      void send(question);
    }
  };
</script>

<section
  class="print-ai-gotchi-chat"
  aria-label={t('print-ai-gotchi.chat.title', { name })}
>
  <div bind:this={log} class="print-ai-gotchi-log" aria-live="polite">
    {#if roast}
      <div class="print-ai-gotchi-turn" data-role="pet">
        <span class="print-ai-gotchi-avatar" aria-hidden="true">
          <PetFace
            face="proud"
            species={view.profile.species}
            stage={view.vitals?.stage}
            size={36}
            label=""
          />
        </span>
        <div class="print-ai-gotchi-said" data-roast="true">
          <p class="print-ai-gotchi-roast-title">
            {t('print-ai-gotchi.roast.title', { name })}
          </p>
          <p>{roast.text}</p>
        </div>
      </div>
    {/if}

    {#if pet.bubbles.length === 0 && (!roast || !awake)}
      <div class="print-ai-gotchi-empty">
        {#if awake}
          <p class="text-base font-medium">
            {t('print-ai-gotchi.chat.hello', { name })}
          </p>
          <p class="text-sm text-muted-foreground">
            {t('print-ai-gotchi.chat.empty', { name })}
          </p>
        {:else}
          <p class="text-base font-medium">
            {t('print-ai-gotchi.chat.asleep', { name })}
          </p>
          <p class="text-sm text-muted-foreground">
            {t('print-ai-gotchi.chat.asleepHint')}
          </p>
          <div>
            <Button onclick={onSetup}>{t('print-ai-gotchi.chat.setup')}</Button>
          </div>
        {/if}
      </div>
    {/if}

    {#each pet.bubbles as bubble, index (index)}
      {#if bubble.role === 'assistant'}
        <div class="print-ai-gotchi-turn" data-role="pet">
          <span class="print-ai-gotchi-avatar" aria-hidden="true">
            <PetFace
              face={bubble.failed ? 'worried' : view.mood.face}
              species={view.profile.species}
              stage={view.vitals?.stage}
              talking={bubble.pending === true && bubble.content !== ''}
              size={36}
              label=""
            />
          </span>
          <div
            class="print-ai-gotchi-said"
            data-failed={bubble.failed === true}
          >
            {#if bubble.pending && !bubble.content}
              <span
                class="print-ai-gotchi-typing"
                role="status"
                aria-label={t('print-ai-gotchi.chat.typing', { name })}
              >
                <span></span><span></span><span></span>
              </span>
            {:else if bubble.failed}
              {t('print-ai-gotchi.chat.failed', {
                name,
                error: bubble.content,
              })}
            {:else}
              {bubble.content}{#if bubble.pending}<span
                  class="print-ai-gotchi-caret"
                ></span>{/if}
            {/if}
          </div>
        </div>
      {:else}
        <div class="print-ai-gotchi-turn" data-role="person">
          <div class="print-ai-gotchi-said">{bubble.content}</div>
        </div>
      {/if}
    {/each}
  </div>

  <div class="print-ai-gotchi-compose">
    {#if awake && !pet.thinking}
      <div class="flex flex-wrap gap-2">
        {#each suggestions as s (s.key)}
          <Chip
            onclick={() =>
              void send(t(`print-ai-gotchi.chat.suggest.${s.key}`, s.params))}
          >
            {t(`print-ai-gotchi.chat.suggest.${s.key}`, s.params)}
          </Chip>
        {/each}
      </div>
    {/if}

    <form
      class="print-ai-gotchi-input"
      onsubmit={(event) => {
        event.preventDefault();
        void send(question);
      }}
    >
      <Textarea
        id="{uid}-question"
        bind:value={question}
        rows={1}
        disabled={!awake}
        placeholder={t('print-ai-gotchi.chat.placeholder', { name })}
        aria-label={t('print-ai-gotchi.chat.placeholder', { name })}
        onkeydown={onKey}
      />
      <div class="flex items-center gap-1">
        {#if pet.bubbles.length > 0}
          <Button
            variant="ghost"
            size="icon-md"
            label={t('print-ai-gotchi.chat.forget')}
            onclick={() => pet.forget()}
          >
            <RotateCcwIcon class="size-4" />
          </Button>
        {/if}
        <Button
          type="submit"
          size="icon-md"
          label={t('print-ai-gotchi.chat.send')}
          disabled={!awake || pet.thinking || !question.trim()}
        >
          <ArrowUpIcon class="size-4" />
        </Button>
      </div>
    </form>
  </div>
</section>

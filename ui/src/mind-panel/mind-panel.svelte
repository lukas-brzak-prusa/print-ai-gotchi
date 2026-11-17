<script lang="ts">
  // Where the pet thinks: pick a provider, then a local model or a remote
  // model and its key.
  import CloudIcon from '@lucide/svelte/icons/cloud';
  import CpuIcon from '@lucide/svelte/icons/cpu';
  import ServerIcon from '@lucide/svelte/icons/server';
  import { Badge, Spinner, Tile } from '@prusa/connect-ui/primitives';
  import { onMount } from 'svelte';

  import type { Provider } from '../api';
  import { heldHost } from '../host';
  import { heldPet } from '../pet.svelte';
  import LocalModels from './local-models.svelte';
  import RemoteForm from './remote-form.svelte';

  const t = heldHost().t;
  const pet = heldPet();

  onMount(() => pet.watchMind());

  const mind = $derived(pet.mind);
  // The tile a person looks at. It starts on the provider in use, and moving
  // it does not change the brain until they choose a model there.
  let shown = $state<string | undefined>();
  const current = $derived(shown ?? mind?.choice.provider);
  const provider = $derived(mind?.providers.find((p) => p.id === current));

  const iconOf = (p: Provider) => {
    if (p.kind === 'local') return CpuIcon;
    return p.on_premises ? ServerIcon : CloudIcon;
  };
</script>

{#if !mind}
  <p class="flex items-center gap-2 text-sm text-muted-foreground">
    <Spinner />
    {t('print-ai-gotchi.page.loading')}
  </p>
{:else}
  <div class="flex flex-col gap-6">
    {#if !mind.editable}
      <p class="rounded-md border px-3 py-2 text-sm text-muted-foreground">
        {t('print-ai-gotchi.mind.locked')}
      </p>
    {/if}

    <section class="flex flex-col gap-3">
      <h2 class="text-base font-semibold">{t('print-ai-gotchi.mind.where')}</h2>
      <div class="print-ai-gotchi-providers">
        {#each mind.providers as p (p.id)}
          {@const Icon = iconOf(p)}
          <Tile
            selected={current === p.id}
            label={t(`print-ai-gotchi.provider.${p.id}.title`)}
            onclick={() => (shown = p.id)}
          >
            <span class="flex flex-col gap-1 text-start">
              <span class="flex items-center justify-between gap-2">
                <Icon class="size-4 text-muted-foreground" />
                {#if mind.choice.provider === p.id}
                  <Badge variant={mind.ready ? 'success' : 'secondary'}>
                    {mind.ready
                      ? t('print-ai-gotchi.mind.inUse')
                      : t('print-ai-gotchi.mind.chosen')}
                  </Badge>
                {/if}
              </span>
              <span class="text-sm font-medium"
                >{t(`print-ai-gotchi.provider.${p.id}.title`)}</span
              >
              <span class="text-xs text-muted-foreground">
                {t(`print-ai-gotchi.provider.${p.id}.line`)}
              </span>
            </span>
          </Tile>
        {/each}
      </div>
    </section>

    {#if provider?.kind === 'local'}
      <LocalModels {mind} />
    {:else if provider}
      {#key provider.id}
        <RemoteForm {mind} {provider} />
      {/key}
    {/if}
  </div>
{/if}

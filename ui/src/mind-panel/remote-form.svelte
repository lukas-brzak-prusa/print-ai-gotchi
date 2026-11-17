<script lang="ts">
  // A brain somewhere else: its address, its key, and the model, picked from
  // what the provider itself lists once the key works.
  import CheckIcon from '@lucide/svelte/icons/check';
  import PlugIcon from '@lucide/svelte/icons/plug';
  import {
    Button,
    Combobox,
    Input,
    Label,
    Spinner,
  } from '@prusa/connect-ui/primitives';

  import type { Mind, ProbeResult, Provider } from '../api';
  import { heldHost } from '../host';
  import { heldPet } from '../pet.svelte';

  interface Props {
    mind: Mind;
    provider: Provider;
  }

  let { mind, provider }: Props = $props();

  const t = heldHost().t;
  const pet = heldPet();
  const uid = $props.id();

  type Typed = Event & { currentTarget: HTMLInputElement };

  // svelte-ignore state_referenced_locally
  let url = $state(mind.choice.base_url[provider.id] ?? provider.base_url);
  // svelte-ignore state_referenced_locally
  let model = $state(mind.choice.model[provider.id] ?? '');
  let key = $state('');
  let probing = $state(false);
  let probed = $state<ProbeResult | undefined>();
  let saved = $state(false);

  // svelte-ignore state_referenced_locally
  const hasKey = mind.choice.has_key[provider.id] === true;
  const inUse = $derived(mind.choice.provider === provider.id);

  const items = $derived(
    [...new Set([...(probed?.models ?? []), ...(model ? [model] : [])])].map(
      (value) => ({ value, label: value }),
    ),
  );

  const probe = async (): Promise<void> => {
    probing = true;
    saved = false;
    probed = await pet.probe({
      provider: provider.id,
      ...(provider.editable_url ? { base_url: url } : {}),
      ...(key ? { key } : {}),
    });
    probing = false;
    if (probed.ok && !model && probed.models[0]) model = probed.models[0];
  };

  const save = async (): Promise<void> => {
    saved = await pet.choose({
      provider: provider.id,
      model,
      ...(provider.editable_url ? { base_url: url } : {}),
      ...(key ? { key } : {}),
    });
    if (saved) key = '';
  };

  const canSave = $derived(
    mind.editable &&
      !pet.mindBusy &&
      model.trim() !== '' &&
      (!provider.needs_key || hasKey || key.trim() !== '') &&
      (!provider.editable_url || url.trim() !== ''),
  );
</script>

<section class="flex max-w-xl flex-col gap-4">
  <div class="flex flex-col gap-1">
    <h2 class="text-base font-semibold">
      {t(`print-ai-gotchi.provider.${provider.id}.title`)}
    </h2>
    <p class="text-sm text-muted-foreground">
      {provider.on_premises
        ? t('print-ai-gotchi.remote.onPremises')
        : t('print-ai-gotchi.remote.cloud')}
    </p>
  </div>

  {#if provider.editable_url}
    <div class="flex flex-col gap-1">
      <Label for="{uid}-url">{t('print-ai-gotchi.remote.url')}</Label>
      <Input
        id="{uid}-url"
        value={url}
        inputmode="url"
        placeholder={provider.base_url ||
          t('print-ai-gotchi.remote.urlExample')}
        oninput={(event: Typed) => {
          url = event.currentTarget.value;
        }}
      />
    </div>
  {/if}

  {#if provider.needs_key || provider.id === 'custom'}
    <div class="flex flex-col gap-1">
      <Label for="{uid}-key">{t('print-ai-gotchi.remote.key')}</Label>
      <Input
        id="{uid}-key"
        type="password"
        autocomplete="off"
        value={key}
        placeholder={hasKey ? t('print-ai-gotchi.remote.keyKept') : ''}
        oninput={(event: Typed) => {
          key = event.currentTarget.value;
        }}
      />
      {#if provider.key_url}
        <a
          class="text-xs text-primary underline-offset-4 hover:underline"
          href={provider.key_url}
          target="_blank"
          rel="noreferrer"
        >
          {t('print-ai-gotchi.remote.getKey', {
            provider: t(`print-ai-gotchi.provider.${provider.id}.title`),
          })}
        </a>
      {/if}
    </div>
  {/if}

  <div class="flex items-center gap-3">
    <Button
      variant="secondary"
      disabled={probing || !mind.editable}
      onclick={() => void probe()}
    >
      {#if probing}<Spinner />{:else}<PlugIcon class="size-4" />{/if}
      {t('print-ai-gotchi.remote.test')}
    </Button>
    {#if probed?.ok}
      <span class="flex items-center gap-1 text-sm text-success">
        <CheckIcon class="size-4" />
        {t('print-ai-gotchi.remote.ok', { count: probed.models.length })}
      </span>
    {/if}
  </div>
  {#if probed && !probed.ok}
    <p class="text-sm break-words text-destructive">{probed.error}</p>
  {/if}

  <div class="flex flex-col gap-1">
    <Label for="{uid}-model">{t('print-ai-gotchi.remote.model')}</Label>
    <Combobox
      id="{uid}-model"
      {items}
      value={model || undefined}
      label={t('print-ai-gotchi.remote.model')}
      placeholder={t('print-ai-gotchi.remote.modelHint')}
      onPick={(value: string | undefined) => {
        model = value ?? '';
      }}
      onCreate={(text: string) => {
        model = text.trim();
      }}
      createLabel={(text: string) =>
        t('print-ai-gotchi.remote.useModel', { model: text })}
    />
  </div>

  <div class="flex items-center gap-3">
    <Button disabled={!canSave} onclick={() => void save()}>
      {inUse
        ? t('print-ai-gotchi.remote.save')
        : t('print-ai-gotchi.remote.choose')}
    </Button>
    {#if saved}
      <span class="text-sm text-muted-foreground">
        {t('print-ai-gotchi.remote.saved', {
          name: t(`print-ai-gotchi.provider.${provider.id}.title`),
        })}
      </span>
    {/if}
  </div>
</section>

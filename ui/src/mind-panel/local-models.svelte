<script lang="ts">
  // The models that run on this server: download one, use it, delete it.
  import DownloadIcon from '@lucide/svelte/icons/download';
  import XIcon from '@lucide/svelte/icons/x';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';
  import {
    Badge,
    Button,
    Progress,
    Spinner,
  } from '@prusa/connect-ui/primitives';

  import type { LocalModel, Mind } from '../api';
  import { heldHost } from '../host';
  import { heldPet } from '../pet.svelte';
  import { sizeText } from '../words';

  interface Props {
    mind: Mind;
  }

  let { mind }: Props = $props();

  const t = heldHost().t;
  const pet = heldPet();

  const installed = $derived(new Set(mind.local.installed ?? []));
  const download = $derived(mind.local.download);
  const usingLocal = $derived(mind.choice.provider === 'local');
  const tooBig = (m: LocalModel): boolean =>
    mind.local.budget > 0 && m.memory > mind.local.budget;

  const use = (m: LocalModel): Promise<boolean> =>
    pet.choose({ provider: 'local', local_model: m.key });

  // The pet keeps its brain while a model downloads and moves to the new one
  // when it lands, so a cancelled download leaves it able to talk.
  const get = async (m: LocalModel): Promise<void> => {
    await pet.install(m.key);
  };
</script>

<section class="flex flex-col gap-3">
  <div class="flex flex-col gap-1">
    <h2 class="text-base font-semibold">{t('print-ai-gotchi.local.title')}</h2>
    <p class="max-w-prose text-sm text-muted-foreground">
      {t('print-ai-gotchi.local.intro')}
    </p>
  </div>

  {#if mind.local.state === 'no_data'}
    <p class="text-sm text-destructive">{t('print-ai-gotchi.local.noData')}</p>
  {:else if !mind.local.supported}
    <p class="text-sm text-destructive">
      {t('print-ai-gotchi.local.unsupported', {
        platform: mind.local.platform,
      })}
    </p>
  {/if}

  {#if usingLocal && mind.local.state === 'failed'}
    <p class="text-sm break-words text-destructive">
      {t('print-ai-gotchi.local.failed', { error: mind.local.error ?? '' })}
    </p>
  {/if}

  <ul class="print-ai-gotchi-models">
    {#each mind.models as m (m.key)}
      {@const isUsed = usingLocal && mind.choice.local_model === m.key}
      {@const isHere = installed.has(m.key)}
      {@const loading = download?.running && download.model === m.key}
      <li data-used={isUsed}>
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          <p class="flex flex-wrap items-center gap-2">
            <span class="font-medium">{m.title}</span>
            <span class="text-xs text-muted-foreground">{m.maker}</span>
            {#if m.multilingual}
              <Badge variant="info"
                >{t('print-ai-gotchi.local.multilingual')}</Badge
              >
            {:else}
              <Badge variant="outline"
                >{t('print-ai-gotchi.local.english')}</Badge
              >
            {/if}
          </p>
          <p class="text-xs text-muted-foreground">
            {t('print-ai-gotchi.local.facts', {
              size: sizeText(m.size),
              memory: sizeText(m.memory),
              for: t(`print-ai-gotchi.for.${m.for}`),
            })}
            {#if !m.measured}{t('print-ai-gotchi.local.estimated')}{/if}
            <a
              class="ms-2 underline underline-offset-2"
              href={m.license_url}
              target="_blank"
              rel="noreferrer"
            >
              {m.license}
            </a>
          </p>
          {#if tooBig(m)}
            <p class="text-xs text-destructive">
              {t('print-ai-gotchi.local.tooBig', {
                budget: sizeText(mind.local.budget),
              })}
            </p>
          {/if}
          {#if loading && download}
            <div class="flex flex-col gap-1 pt-1">
              <Progress
                value={download.done}
                max={Math.max(download.total, 1)}
              />
              <p class="text-xs text-muted-foreground tabular-nums">
                {t(`print-ai-gotchi.local.step.${download.step || 'model'}`, {
                  done: sizeText(download.done),
                  total: sizeText(download.total),
                })}
              </p>
            </div>
          {:else if download && !download.running && download.model === m.key && download.error}
            <p class="text-xs break-words text-destructive">{download.error}</p>
          {/if}
        </div>

        <div class="flex shrink-0 items-center gap-2">
          {#if isUsed && mind.local.state === 'starting'}
            <span class="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner />
              {t('print-ai-gotchi.local.starting')}
            </span>
          {:else if isUsed && mind.local.state === 'ready'}
            <Badge variant="success">{t('print-ai-gotchi.mind.inUse')}</Badge>
          {:else if isHere}
            <Button
              variant="secondary"
              size="sm"
              disabled={!mind.editable || pet.mindBusy}
              onclick={() => void use(m)}
            >
              {t('print-ai-gotchi.local.use')}
            </Button>
          {:else if loading}
            <Button
              variant="secondary"
              size="sm"
              disabled={!mind.editable || pet.mindBusy}
              onclick={() => void pet.cancelDownload()}
            >
              <XIcon class="size-4" />
              {t('print-ai-gotchi.local.cancel')}
            </Button>
          {:else}
            <Button
              size="sm"
              disabled={!mind.editable ||
                pet.mindBusy ||
                download?.running === true ||
                !mind.local.supported ||
                mind.local.state === 'no_data'}
              onclick={() => void get(m)}
            >
              <DownloadIcon class="size-4" />
              {t('print-ai-gotchi.local.download')}
            </Button>
          {/if}
          {#if isHere}
            <Button
              variant="ghost"
              size="icon-md"
              label={t('print-ai-gotchi.local.remove', { model: m.title })}
              disabled={!mind.editable || pet.mindBusy}
              onclick={() => void pet.remove(m.key)}
            >
              <Trash2Icon class="size-4" />
            </Button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>
</section>

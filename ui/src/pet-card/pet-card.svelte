<script lang="ts">
  // The pet on a dashboard: its face, what it says, and a way to its page.
  // A card polls nothing of its own. It reads the pet once, and again when the
  // fleet the app already follows changes, at most every few seconds.
  import type { CardProps } from '@prusa/connect-ui/slots';

  import { heldHost } from '../host';
  import { heldPet } from '../pet.svelte';
  import PetFace from '../pet-face/pet-face.svelte';
  import { latestQuip, noticeText } from '../words';

  let { title, size, editing }: CardProps = $props();

  const host = heldHost();
  const t = host.t;
  const pet = heldPet();

  const REFRESH_MS = 5000;
  let last = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    // Every change to the fleet the app holds is a reason to look again.
    if (host.fleet.devices.length < 0) return;
    const wait = Math.max(0, last + REFRESH_MS - Date.now());
    clearTimeout(timer);
    timer = setTimeout(() => {
      last = Date.now();
      void pet.load();
    }, wait);
    return () => clearTimeout(timer);
  });

  const view = $derived(pet.view);
  const said = $derived(
    view
      ? (latestQuip(view.notices) ??
          t(`print-ai-gotchi.mood.${view.mood.face}`))
      : '',
  );
  const newest = $derived(view?.notices[0]);
  const faceSize = $derived(Math.min(140, Math.max(72, size.h * 28)));
</script>

<div class="flex h-full flex-col gap-2 overflow-hidden p-3">
  <p class="truncate text-sm font-semibold">
    {title || (view?.profile.name ?? t('print-ai-gotchi.card.title'))}
  </p>

  {#if view}
    <div class="flex min-h-0 flex-1 items-center gap-3">
      <PetFace
        face={view.mood.face}
        species={view.profile.species}
        size={faceSize}
        label={t('print-ai-gotchi.face.label', {
          name: view.profile.name,
          face: t(`print-ai-gotchi.face.${view.mood.face}`),
        })}
      />
      <div class="flex min-w-0 flex-1 flex-col gap-2 text-sm">
        <p class="line-clamp-4">{said}</p>
        {#if newest && size.h >= 4}
          <p class="line-clamp-2 text-xs text-muted-foreground">
            {noticeText(t, newest)}
          </p>
        {/if}
      </div>
    </div>

    {#if !editing}
      <a
        class="text-sm font-medium text-primary underline-offset-4 hover:underline"
        href={host.pageHref('print-ai-gotchi')}
      >
        {t('print-ai-gotchi.card.open', { name: view.profile.name })}
      </a>
    {/if}
  {:else if pet.error}
    <p class="text-sm text-destructive">{pet.error}</p>
  {/if}
</div>

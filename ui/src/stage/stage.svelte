<script lang="ts">
  // The pet on its print sheet: what it says, the fleet in one line, and the
  // last few things it noticed. This is the one loud part of the page; the
  // rest is the app's own furniture.
  import type { PetView } from '../api';
  import { heldHost } from '../host';
  import PetFace from '../pet-face/pet-face.svelte';
  import { latestQuip, noticeText, timeText } from '../words';

  interface Props {
    view: PetView;
    talking: boolean;
  }

  let { view, talking }: Props = $props();

  const t = heldHost().t;

  const said = $derived(
    latestQuip(view.notices) ?? t(`print-ai-gotchi.mood.${view.mood.face}`),
  );
  const recent = $derived(view.notices.slice(0, 3));
  const counts = $derived(
    (
      [
        ['printers', view.mood.printers],
        ['printing', view.mood.printing],
        ['problems', view.mood.problems],
      ] as const
    ).filter(([key, count]) => key === 'printers' || count > 0),
  );
</script>

<section class="print-ai-gotchi-stage" data-face={view.mood.face}>
  <div class="print-ai-gotchi-bubble">
    <p>{said}</p>
  </div>

  <div class="print-ai-gotchi-stage-pet">
    <PetFace
      face={view.mood.face}
      species={view.profile.species}
      {talking}
      size={208}
      label={t('print-ai-gotchi.face.label', {
        name: view.profile.name,
        face: t(`print-ai-gotchi.face.${view.mood.face}`),
      })}
    />
  </div>

  <div class="print-ai-gotchi-stage-foot">
    <p class="print-ai-gotchi-counts">
      {#each counts as [key, count] (key)}
        <span data-key={key}
          >{t(`print-ai-gotchi.stats.${key}`, { count })}</span
        >
      {/each}
    </p>
    {#if recent.length > 0}
      <ol class="print-ai-gotchi-recent">
        {#each recent as notice (notice.id)}
          <li data-severity={notice.severity}>
            <time datetime={notice.at}>{timeText(notice.at)}</time>
            <span>{noticeText(t, notice)}</span>
          </li>
        {/each}
      </ol>
    {/if}
  </div>
</section>

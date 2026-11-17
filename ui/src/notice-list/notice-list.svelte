<script lang="ts">
  // Everything the pet noticed, newest first, with its own comment under each.
  import {
    Badge,
    Card,
    CardContent,
    CardHeader,
    CardTitle,
  } from '@prusa/connect-ui/primitives';

  import type { Notice } from '../api';
  import { heldHost } from '../host';
  import { noticeText, timeText } from '../words';

  interface Props {
    notices: readonly Notice[];
    name: string;
  }

  let { notices, name }: Props = $props();

  const t = heldHost().t;

  const variant = {
    error: 'destructive',
    warning: 'warning',
    info: 'secondary',
  } as const;
</script>

<Card>
  <CardHeader>
    <CardTitle>{t('print-ai-gotchi.notices.title', { name })}</CardTitle>
  </CardHeader>
  <CardContent>
    {#if notices.length === 0}
      <p class="text-sm text-muted-foreground">
        {t('print-ai-gotchi.notices.empty', { name })}
      </p>
    {:else}
      <ol class="flex flex-col divide-y">
        {#each notices as notice (notice.id)}
          <li class="flex gap-3 py-2 text-sm">
            <time
              class="w-12 shrink-0 text-muted-foreground tabular-nums"
              datetime={notice.at}
            >
              {timeText(notice.at)}
            </time>
            <div class="flex min-w-0 flex-1 flex-col gap-1">
              <p class="flex flex-wrap items-center gap-2">
                <Badge variant={variant[notice.severity]}
                  >{t(`print-ai-gotchi.severity.${notice.severity}`)}</Badge
                >
                <span class="break-words">{noticeText(t, notice)}</span>
              </p>
              {#if notice.quip}
                <p class="text-muted-foreground italic">{notice.quip}</p>
              {/if}
            </div>
          </li>
        {/each}
      </ol>
    {/if}
  </CardContent>
</Card>

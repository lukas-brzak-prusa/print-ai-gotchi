<script lang="ts">
  // The pet's own page: the pet and the conversation first, then its brain,
  // its character and what it noticed, each on a tab of its own.
  import CloudIcon from '@lucide/svelte/icons/cloud';
  import CpuIcon from '@lucide/svelte/icons/cpu';
  import {
    Spinner,
    Tabs,
    TabsContent,
    TabsList,
    TabsTrigger,
  } from '@prusa/connect-ui/primitives';
  import { onMount } from 'svelte';

  import ChatPanel from './chat-panel/chat-panel.svelte';
  import { heldHost } from './host';
  import MindPanel from './mind-panel/mind-panel.svelte';
  import NoticeList from './notice-list/notice-list.svelte';
  import { heldPet } from './pet.svelte';
  import ProfileForm from './profile-form/profile-form.svelte';
  import Stage from './stage/stage.svelte';

  const t = heldHost().t;
  const pet = heldPet();

  let tab = $state('talk');

  onMount(() => pet.watch());

  const brain = $derived(pet.view?.brain);
  const brainLine = $derived.by(() => {
    if (!brain) return '';
    if (!brain.ready) {
      return brain.state === 'starting'
        ? t('print-ai-gotchi.header.waking')
        : t('print-ai-gotchi.header.noBrain');
    }
    return t('print-ai-gotchi.header.brain', {
      model: brain.model,
      where: t(`print-ai-gotchi.provider.${brain.provider}.title`),
    });
  });
</script>

<div class="flex flex-col gap-4">
  <header class="flex flex-wrap items-end justify-between gap-3">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold">
        {pet.view?.profile.name ?? t('print-ai-gotchi.page.title')}
      </h1>
      {#if brain}
        <p class="flex items-center gap-2 text-sm text-muted-foreground">
          {#if brain.on_premises}
            <CpuIcon class="size-4" />
          {:else}
            <CloudIcon class="size-4" />
          {/if}
          <span>{brainLine}</span>
          {#if brain.ready}
            <span
              class="print-ai-gotchi-dot"
              data-on={brain.on_premises}
              aria-hidden="true"
            ></span>
          {/if}
        </p>
      {/if}
    </div>
  </header>

  {#if pet.error}
    <p
      class="rounded-md border border-destructive px-3 py-2 text-sm text-destructive"
    >
      {pet.error}
    </p>
  {/if}

  {#if pet.view}
    <Tabs bind:value={tab}>
      <TabsList>
        <TabsTrigger value="talk">{t('print-ai-gotchi.tab.talk')}</TabsTrigger>
        <TabsTrigger value="brain">{t('print-ai-gotchi.tab.brain')}</TabsTrigger
        >
        <TabsTrigger value="character"
          >{t('print-ai-gotchi.tab.character')}</TabsTrigger
        >
        <TabsTrigger value="journal"
          >{t('print-ai-gotchi.tab.journal')}</TabsTrigger
        >
      </TabsList>

      <TabsContent value="talk">
        <div class="print-ai-gotchi-talk">
          <Stage view={pet.view} talking={pet.thinking} />
          <ChatPanel view={pet.view} onSetup={() => (tab = 'brain')} />
        </div>
      </TabsContent>

      <TabsContent value="brain">
        <div class="pt-2"><MindPanel /></div>
      </TabsContent>

      <TabsContent value="character">
        {#key JSON.stringify(pet.view.profile)}
          <ProfileForm view={pet.view} />
        {/key}
      </TabsContent>

      <TabsContent value="journal">
        <NoticeList notices={pet.view.notices} name={pet.view.profile.name} />
      </TabsContent>
    </Tabs>
  {:else if !pet.error}
    <p class="flex items-center gap-2 text-sm text-muted-foreground">
      <Spinner />
      {t('print-ai-gotchi.page.loading')}
    </p>
  {/if}
</div>

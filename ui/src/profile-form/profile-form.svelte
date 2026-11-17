<script lang="ts">
  // Who the pet is: its name, its character, the language it speaks and how it looks.
  import {
    Button,
    Input,
    Label,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
  } from '@prusa/connect-ui/primitives';

  import type { PetView, Species } from '../api';
  import { heldHost } from '../host';
  import { heldPet } from '../pet.svelte';
  import PetFace from '../pet-face/pet-face.svelte';

  interface Props {
    view: PetView;
  }

  let { view }: Props = $props();

  const t = heldHost().t;
  const pet = heldPet();
  const uid = $props.id();

  type Typed = Event & { currentTarget: HTMLInputElement };

  // The form starts from what the server holds and keeps its own copy, so a
  // refresh of the pet while somebody types does not wipe what they typed.
  // svelte-ignore state_referenced_locally
  let draft = $state({ ...view.profile });

  const fields = $derived([
    {
      key: 'personality' as const,
      items: view.choices.personalities.map((value) => ({
        value,
        label: t(`print-ai-gotchi.personality.${value}`),
      })),
    },
    {
      key: 'language' as const,
      items: view.choices.languages.map((value) => ({
        value,
        label: t(`print-ai-gotchi.language.${value}`),
      })),
    },
    {
      key: 'species' as const,
      items: view.choices.species.map((value) => ({
        value,
        label: t(`print-ai-gotchi.species.${value}`),
      })),
    },
  ]);

  const changed = $derived(
    draft.name !== view.profile.name ||
      draft.personality !== view.profile.personality ||
      draft.language !== view.profile.language ||
      draft.species !== view.profile.species,
  );
</script>

<div class="print-ai-gotchi-character">
  <div class="print-ai-gotchi-stage print-ai-gotchi-preview" data-face="proud">
    <PetFace
      face="proud"
      species={draft.species}
      size={176}
      label={t('print-ai-gotchi.profile.preview', { name: draft.name })}
    />
    <p class="print-ai-gotchi-preview-name">{draft.name}</p>
    <p class="print-ai-gotchi-preview-line">
      {t(`print-ai-gotchi.personality.${draft.personality}.line`)}
    </p>
  </div>
  <form
    class="flex max-w-md flex-col gap-3"
    onsubmit={(event) => {
      event.preventDefault();
      void pet.saveProfile(draft);
    }}
  >
    <div class="flex flex-col gap-1">
      <Label for="{uid}-name">{t('print-ai-gotchi.profile.name')}</Label>
      <Input
        id="{uid}-name"
        value={draft.name}
        maxlength={24}
        required
        oninput={(event: Typed) => {
          draft.name = event.currentTarget.value;
        }}
      />
    </div>

    {#each fields as field (field.key)}
      <div class="flex flex-col gap-1">
        <Label for="{uid}-{field.key}"
          >{t(`print-ai-gotchi.profile.${field.key}`)}</Label
        >
        <Select
          value={draft[field.key]}
          onValueChange={(value: string) => {
            if (field.key === 'species') draft.species = value as Species;
            else draft[field.key] = value;
          }}
          items={field.items}
        >
          <SelectTrigger id="{uid}-{field.key}">
            <SelectValue>
              {field.items.find((item) => item.value === draft[field.key])
                ?.label}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {#each field.items as item (item.value)}
              <SelectItem value={item.value} label={item.label}
                >{item.label}</SelectItem
              >
            {/each}
          </SelectContent>
        </Select>
      </div>
    {/each}

    <div>
      <Button type="submit" disabled={!changed || pet.saving}
        >{t('print-ai-gotchi.profile.save')}</Button
      >
    </div>
  </form>
</div>

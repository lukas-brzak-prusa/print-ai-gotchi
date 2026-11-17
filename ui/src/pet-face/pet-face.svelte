<script lang="ts">
  // The pet, drawn. Three species share one face, and the face follows the
  // mood the server worked out from the printers. Every shape is decoration:
  // the label on the svg says what the picture means.
  import type { Face, Species, Stage } from '../api';

  interface Props {
    face: Face;
    species: Species;
    label: string;
    /** How far the pet grew: a baby is small and still in its shell, a
     * legend wears a crown. */
    stage?: Stage | undefined;
    /** The pet is talking: the mouth moves. */
    talking?: boolean;
    size?: number;
  }

  let {
    face,
    species,
    label,
    stage = 'adult',
    talking = false,
    size = 160,
  }: Props = $props();

  // A young pet is drawn smaller, standing on the same ground.
  const scale = $derived(
    ({ baby: 0.72, kid: 0.86, adult: 1, legend: 1 } as const)[stage],
  );
  const grown = $derived(
    `translate(100 186) scale(${String(scale)}) translate(-100 -186)`,
  );

  const eyesOpen = $derived(
    face === 'alarmed' ||
      face === 'worried' ||
      face === 'busy' ||
      face === 'hungry',
  );
</script>

<svg
  class="print-ai-gotchi-face"
  data-face={face}
  data-species={species}
  data-talking={talking}
  data-stage={stage}
  viewBox="0 0 200 200"
  width={size}
  height={size}
  role="img"
  aria-label={label}
>
  <g transform={grown}>
    <g class="pp-body">
      {#if species === 'cat'}
        <!-- the heated bed it sleeps on -->
        <rect class="pp-bed" x="22" y="166" width="156" height="12" rx="4" />
        <path
          class="pp-heat"
          d="M60 190 q4 -6 0 -10 M100 192 q4 -6 0 -10 M140 190 q4 -6 0 -10"
        />
        <path class="pp-tail-edge" d="M150 150 C 190 150, 190 100, 168 92" />
        <path class="pp-tail" d="M150 150 C 190 150, 190 100, 168 92" />
        <path class="pp-fill" d="M48 92 L56 42 L88 68 Z" />
        <path class="pp-fill" d="M152 92 L144 42 L112 68 Z" />
        <path class="pp-inner" d="M56 80 L60 54 L78 68 Z" />
        <path class="pp-inner" d="M144 80 L140 54 L122 68 Z" />
        <ellipse class="pp-fill" cx="100" cy="114" rx="64" ry="54" />
        <path
          class="pp-line"
          d="M40 118 L66 122 M40 130 L66 128 M160 118 L134 122 M160 130 L134 128"
        />
      {:else if species === 'robot'}
        <line class="pp-line" x1="100" y1="58" x2="100" y2="30" />
        <circle class="pp-bulb" cx="100" cy="26" r="8" />
        <rect class="pp-fill" x="42" y="58" width="116" height="100" rx="24" />
        <rect class="pp-inner" x="56" y="76" width="88" height="66" rx="14" />
        <path class="pp-nozzle" d="M86 158 H114 L106 178 H94 Z" />
        <circle class="pp-drip" cx="100" cy="186" r="3" />
      {:else}
        <path
          class="pp-sprout"
          d="M100 58 C 92 40, 112 34, 104 20 C 98 10, 112 6, 116 14"
        />
        <path
          class="pp-fill pp-wobble"
          d="M100 56 C 150 52, 178 96, 170 132 C 162 170, 128 178, 100 176 C 70 178, 34 168, 30 132 C 24 92, 52 58, 100 56 Z"
        />
      {/if}
    </g>

    <g class="pp-face">
      {#if eyesOpen}
        <g class="pp-eyes">
          {#if face === 'hungry'}
            <!-- big eyes looking up, for food -->
            <circle class="pp-white" cx="78" cy="108" r="10" />
            <circle class="pp-white" cx="122" cy="108" r="10" />
            <circle class="pp-ink" cx="79" cy="102" r="4.5" />
            <circle class="pp-ink" cx="123" cy="102" r="4.5" />
          {:else if face === 'alarmed'}
            <circle class="pp-white" cx="78" cy="108" r="11" />
            <circle class="pp-white" cx="122" cy="108" r="11" />
            <circle class="pp-ink" cx="78" cy="108" r="3.5" />
            <circle class="pp-ink" cx="122" cy="108" r="3.5" />
          {:else}
            <circle class="pp-ink" cx="78" cy="108" r="6.5" />
            <circle class="pp-ink" cx="122" cy="108" r="6.5" />
            <circle class="pp-white" cx="80" cy="105" r="2" />
            <circle class="pp-white" cx="124" cy="105" r="2" />
          {/if}
        </g>
      {:else if face === 'proud'}
        <path
          class="pp-line"
          d="M68 110 Q78 98 88 110 M112 110 Q122 98 132 110"
        />
      {:else if face === 'sleepy'}
        <path
          class="pp-line"
          d="M68 106 Q78 114 88 106 M112 106 Q122 114 132 106"
        />
      {:else}
        <path class="pp-line" d="M68 106 H88 M112 106 H132" />
        <circle class="pp-ink" cx="78" cy="111" r="4" />
        <circle class="pp-ink" cx="122" cy="111" r="4" />
      {/if}

      {#if face === 'worried' || face === 'hungry'}
        <path class="pp-line" d="M66 92 L88 98 M134 92 L112 98" />
      {:else if face === 'alarmed'}
        <path class="pp-line" d="M66 88 L88 92 M134 88 L112 92" />
      {/if}

      {#if face === 'proud'}
        <circle class="pp-blush" cx="64" cy="126" r="7" />
        <circle class="pp-blush" cx="136" cy="126" r="7" />
      {/if}

      <g class="pp-mouth">
        {#if face === 'alarmed'}
          <ellipse class="pp-ink" cx="100" cy="138" rx="8" ry="10" />
        {:else if face === 'worried'}
          <path class="pp-line" d="M84 140 q4 -5 8 0 t8 0 t8 0" />
        {:else if face === 'proud'}
          <path class="pp-ink" d="M82 130 Q100 152 118 130 Z" />
        {:else if face === 'busy'}
          <path class="pp-line" d="M88 134 Q100 144 112 134" />
        {:else if face === 'hungry'}
          <path class="pp-ink" d="M88 132 Q100 124 112 132 Q100 146 88 132 Z" />
        {:else if face === 'sleepy'}
          <circle class="pp-ink" cx="100" cy="138" r="4" />
        {:else}
          <path class="pp-line" d="M88 138 H112" />
        {/if}
      </g>
    </g>

    {#if stage === 'baby'}
      <!-- the shell it hatched from -->
      <path
        class="pp-shell"
        d="M40 168 L52 150 L64 164 L78 148 L92 164 L106 148 L120 164 L134 148 L148 164 L160 150 L160 168 Q160 186 100 186 Q40 186 40 168 Z"
      />
    {:else if stage === 'legend'}
      <g class="pp-crown">
        <path d="M78 60 L76 38 L89 50 L100 32 L111 50 L124 38 L122 60 Z" />
        <circle cx="100" cy="50" r="3.5" />
      </g>
    {/if}
    {#if face === 'hungry'}
      <path class="pp-drool" d="M108 138 q-3 8 0 12 q3 -4 0 -12 Z" />
    {/if}
  </g>

  {#if face === 'hungry'}
    <!-- the stomach rumbling -->
    <path
      class="pp-rumble"
      d="M150 150 q5 -5 10 0 t10 0 M154 162 q5 -5 10 0 t10 0"
    />
  {:else if face === 'alarmed'}
    <path class="pp-sweat" d="M160 70 q-8 14 0 18 q8 -4 0 -18 Z" />
  {:else if face === 'proud'}
    <path
      class="pp-spark"
      d="M30 50 l4 10 l10 4 l-10 4 l-4 10 l-4 -10 l-10 -4 l10 -4 Z"
    />
    <path
      class="pp-spark pp-spark-late"
      d="M168 40 l3 7 l7 3 l-7 3 l-3 7 l-3 -7 l-7 -3 l7 -3 Z"
    />
  {:else if face === 'sleepy'}
    <path class="pp-z" d="M150 60 h12 l-12 14 h12" />
    <path class="pp-z pp-z-late" d="M168 34 h9 l-9 10 h9" />
  {/if}
</svg>

// The bundle's entry. The app imports this file and calls what it default
// exports, handing it the one object a module receives.
import { defineModule } from '@prusa/connect-ui/slots';

import { holdHost } from './host';
import Page from './page.svelte';
import { holdPet } from './pet.svelte';
import PetCard from './pet-card/pet-card.svelte';
import './styles.css';

export default defineModule((host) => {
  holdHost(host);
  holdPet(host);

  // Every registration is held to `contributes` in module.yaml.
  host.register.page('print-ai-gotchi', Page);
  host.register.card('pet', PetCard);
});

// Runs the module on its own, as the panel shell would host it.
import { standalone } from './lib/standalone.js';
import { mount } from './module.js';

standalone(mount);

// Runs the module on its own, as the panel shell would host it.
import { standalone } from '@lsongdev/panel/standalone.js';
import { mount } from './module.js';

standalone(mount);

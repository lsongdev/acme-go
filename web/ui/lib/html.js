// Preact + HTM: JSX-like templates as tagged template literals, no build.
import { h, render, Fragment } from '../vendor/preact.mjs';
import htm from '../vendor/htm.mjs';

export const html = htm.bind(h);
export { h, render, Fragment };
export {
  useState, useEffect, useMemo, useCallback, useRef, useReducer, useContext, useLayoutEffect,
} from '../vendor/hooks.mjs';

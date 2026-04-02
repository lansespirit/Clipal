import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const root = resolve('/Users/sean/Programs/Clipal/landing-page');
const mainJs = readFileSync(resolve(root, 'js/main.js'), 'utf8');
const enHtml = readFileSync(resolve(root, 'index.html'), 'utf8');
const zhHtml = readFileSync(resolve(root, 'zh/index.html'), 'utf8');

assert(
  /<a href="\/zh\/" class="lang-toggle" id="lang-toggle" aria-label="Switch to Chinese">/.test(enHtml),
  'English page should expose a static link to /zh/ for language switching.'
);

assert(
  !/<button class="lang-toggle" id="lang-toggle"/.test(enHtml),
  'English page should not use a button for language switching.'
);

assert(
  /<a href="\/" class="lang-toggle" id="lang-toggle" aria-label="切换到英文">/.test(zhHtml),
  'Chinese page should expose a static link to / for language switching.'
);

assert(
  !/data-doclink=/.test(enHtml) && !/data-doclink=/.test(zhHtml),
  'HTML pages should not keep JS-driven doc-link markers.'
);

assert(
  !/data-i18n=/.test(enHtml) && !/data-i18n=/.test(zhHtml),
  'HTML pages should not keep JS-driven i18n markers.'
);

const forbiddenJsPatterns = [
  'const DOC_LINKS =',
  'const TRANSLATIONS =',
  'localStorage.getItem(\'clipal-lang\')',
  'localStorage.setItem(\'clipal-lang\'',
  'function updateDocLinks',
  'function applyTranslations',
  'function initI18n',
  'window.location.href = nextLang === \'zh\' ? \'/zh/\' : \'/\'',
];

for (const pattern of forbiddenJsPatterns) {
  assert(!mainJs.includes(pattern), `main.js should not contain legacy language switching logic: ${pattern}`);
}

console.log('static-language-cleanup-check: ok');

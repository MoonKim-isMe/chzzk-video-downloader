import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const appSource = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
const urlTabSource = readFileSync(new URL('../src/features/videos/VideoUrlTab.tsx', import.meta.url), 'utf8');

test('URL tab stays mounted while switching search, downloads and repair tabs', () => {
  const mainStart = appSource.indexOf('<main className="app-content-inner">');
  const mainEnd = appSource.indexOf('</main>', mainStart);
  assert.notEqual(mainStart, -1);
  assert.notEqual(mainEnd, -1);
  const mainMarkup = appSource.slice(mainStart, mainEnd);

  // A hidden wrapper must remain rendered regardless of the active tab;
  // conditionally inserting VideoUrlTab would reset user-entered URL/video.
  assert.match(mainMarkup, /<div\s+className="app-main-tab-view"\s+hidden=\{activeTab !== 'url'\}\s*>\s*<VideoUrlTab/);
  assert.equal((mainMarkup.match(/<VideoUrlTab/g) ?? []).length, 1);
  assert.doesNotMatch(mainMarkup, /activeTab === 'url'\s*\?\s*\(/);
  assert.match(mainMarkup, /activeTab === 'downloads' && \(/);
});

test('segment requests only replace URL results when a new segment navigation is requested', () => {
  assert.match(appSource, /setSegmentRequest\(\(current\) => \(\{ video, sequence: \(current\?\.sequence \?\? 0\) \+ 1 \}\)\)/);
  assert.match(urlTabSource, /useEffect\(\(\) => \{\s*if \(!segmentRequest\) return;/);
  assert.match(urlTabSource, /setVideo\(segmentRequest\.video\)/);
  assert.match(urlTabSource, /\}, \[segmentRequest\]\);/);
  assert.match(urlTabSource, /setVideo\(nextVideo\)/);
  assert.match(urlTabSource, /const requestId = \+\+requestSequence\.current;/);
});

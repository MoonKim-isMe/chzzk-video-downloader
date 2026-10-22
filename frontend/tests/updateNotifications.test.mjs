import test from 'node:test';
import assert from 'node:assert/strict';
import {
  getPostponedUpdateVersion,
  postponeUpdateVersion,
  shouldOpenUpdateModal,
} from '../src/lib/updateNotifications.ts';

test('auto updates are suppressed only for the postponed version', () => {
  assert.equal(shouldOpenUpdateModal('0.2.2', null, false), true);
  assert.equal(shouldOpenUpdateModal('0.2.2', '0.2.2', false), false);
  assert.equal(shouldOpenUpdateModal('0.2.3', '0.2.2', false), true);
  assert.equal(shouldOpenUpdateModal('0.2.2', '0.2.2', true), true);
});

test('postponed version is persisted and restored across app sessions', () => {
  const savedWindow = globalThis.window;
  const values = new Map();
  globalThis.window = {
    localStorage: {
      getItem(key) { return values.get(key) ?? null; },
      setItem(key, value) { values.set(key, value); },
    },
  };
  try {
    assert.equal(getPostponedUpdateVersion(), null);
    postponeUpdateVersion('0.2.2');
    assert.equal(getPostponedUpdateVersion(), '0.2.2');
    assert.equal(shouldOpenUpdateModal('0.2.2', getPostponedUpdateVersion(), false), false);
    assert.equal(shouldOpenUpdateModal('0.2.3', getPostponedUpdateVersion(), false), true);
  } finally {
    globalThis.window = savedWindow;
  }
});

test('storage failures do not prevent displaying updates', () => {
  const savedWindow = globalThis.window;
  globalThis.window = { get localStorage() { throw new Error('storage blocked'); } };
  try {
    assert.equal(getPostponedUpdateVersion(), null);
    assert.doesNotThrow(() => postponeUpdateVersion('0.2.2'));
    assert.equal(shouldOpenUpdateModal('0.2.2', getPostponedUpdateVersion(), false), true);
  } finally {
    globalThis.window = savedWindow;
  }
});

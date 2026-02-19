const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const {
  buildActivityPayload,
  mergeRecentByChannel,
  shouldUseRecent,
  channelKeyFromContext,
  extractUsage,
} = require('../index.js');

test('shouldUseRecent enforces freshness window', () => {
  const now = Date.now();
  assert.equal(shouldUseRecent({ ts: now - 59_000 }, now, 60_000), true);
  assert.equal(shouldUseRecent({ ts: now - 61_000 }, now, 60_000), false);
  assert.equal(shouldUseRecent(null, now, 60_000), false);
});

test('mergeRecentByChannel merges llm metadata into sent payload', () => {
  const now = Date.now();
  const recent = {
    ts: now,
    sessionKey: 'agent:main:abc',
    model: 'gpt-5',
    tokensIn: 120,
    tokensOut: 45,
    projectTag: 'clawtivity',
    userId: 'art',
  };

  const merged = mergeRecentByChannel({
    channelId: 'telegram',
    eventTo: 'user-1',
    conversationId: 'conv-1',
    success: true,
    recent,
    now,
  });

  assert.equal(merged.session_key, 'agent:main:abc');
  assert.equal(merged.model, 'gpt-5');
  assert.equal(merged.tokens_in, 120);
  assert.equal(merged.tokens_out, 45);
  assert.equal(merged.status, 'success');
  assert.equal(merged.channel, 'telegram');
  assert.equal(merged.user_id, 'art');
});

test('buildActivityPayload produces fallback session key when recent context absent', () => {
  const payload = buildActivityPayload({
    sessionKey: '',
    model: '',
    tokensIn: 0,
    tokensOut: 0,
    durationMs: 0,
    projectTag: 'clawtivity',
    channel: 'discord',
    userId: 'u-1',
    status: 'failed',
    toolsUsed: [],
    nowIso: '2026-02-18T00:00:00Z',
    fallbackSessionSeed: 'conv-99',
  });

  assert.equal(payload.session_key, 'channel:discord:conv-99');
  assert.equal(payload.status, 'failed');
  assert.equal(payload.project_tag, 'clawtivity');
  assert.equal(payload.created_at, '2026-02-18T00:00:00Z');
});

test('plugin package metadata exists for openclaw install', () => {
  const pkgPath = path.join(__dirname, '..', 'package.json');
  const raw = fs.readFileSync(pkgPath, 'utf8');
  const pkg = JSON.parse(raw);

  assert.equal(pkg.name, 'clawtivity-activity');
  assert.equal(pkg.main, 'index.js');
  assert.ok(pkg.version);
  assert.deepEqual(pkg.openclaw && pkg.openclaw.extensions, ['./index.js']);
});

test('channelKeyFromContext prefers channelId then messageProvider', () => {
  assert.equal(channelKeyFromContext({ channelId: 'telegram', messageProvider: 'discord' }, {}), 'telegram');
  assert.equal(channelKeyFromContext({ messageProvider: 'discord' }, {}), 'discord');
  assert.equal(channelKeyFromContext({}, { to: 'user-1' }), 'user-1');
});

test('extractUsage supports multiple event usage shapes', () => {
  assert.deepEqual(
    extractUsage({ usage: { input: 10, output: 20 } }),
    { tokensIn: 10, tokensOut: 20 },
  );
  assert.deepEqual(
    extractUsage({ usage: { input_tokens: 7, output_tokens: 9 } }),
    { tokensIn: 7, tokensOut: 9 },
  );
  assert.deepEqual(
    extractUsage({ usage: { prompt_tokens: 3, completion_tokens: 4 } }),
    { tokensIn: 3, tokensOut: 4 },
  );
});

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const {
  buildActivityPayload,
  mergeRecentByChannel,
  shouldUseRecent,
  channelKeyFromContext,
  extractUsage,
  extractCognition,
  statusFromSuccess,
  coalesceSnapshot,
  settleSnapshot,
  postWithRetry,
  sendToApi,
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

test('buildActivityPayload carries text signals for classifier', () => {
  const payload = buildActivityPayload({
    sessionKey: 's-1',
    model: 'gpt-5',
    tokensIn: 1,
    tokensOut: 1,
    durationMs: 10,
    projectTag: 'clawtivity',
    channel: 'webchat',
    userId: 'u-1',
    status: 'success',
    toolsUsed: [],
    promptText: 'please research options',
    assistantText: 'here are the findings',
    nowIso: '2026-02-18T00:00:00Z',
  });

  assert.equal(payload.prompt_text, 'please research options');
  assert.equal(payload.assistant_text, 'here are the findings');
});

test('buildActivityPayload carries thinking and reasoning fields', () => {
  const payload = buildActivityPayload({
    sessionKey: 's-2',
    model: 'gpt-5',
    tokensIn: 1,
    tokensOut: 1,
    durationMs: 10,
    projectTag: 'clawtivity',
    channel: 'webchat',
    userId: 'u-1',
    status: 'success',
    toolsUsed: [],
    thinking: 'high',
    reasoning: true,
    nowIso: '2026-02-18T00:00:00Z',
  });

  assert.equal(payload.thinking, 'high');
  assert.equal(payload.reasoning, true);
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

test('plugin source does not use child_process', () => {
  const pluginPath = path.join(__dirname, '..', 'index.js');
  const source = fs.readFileSync(pluginPath, 'utf8');
  assert.equal(source.includes('child_process'), false);
});

test('plugin source does not read queue files', () => {
  const pluginPath = path.join(__dirname, '..', 'index.js');
  const source = fs.readFileSync(pluginPath, 'utf8');
  assert.equal(source.includes('readFileSync'), false);
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

test('extractCognition resolves explicit reasoning fields', () => {
  const got = extractCognition(
    {
      reasoning: {
        enabled: true,
        effort: 'high',
      },
    },
    {},
    {},
  );

  assert.equal(got.thinking, 'high');
  assert.equal(got.reasoning, true);
});

test('extractCognition falls back to low/false when unknown', () => {
  const got = extractCognition({}, {}, {});
  assert.equal(got.thinking, 'low');
  assert.equal(got.reasoning, false);
});

test('statusFromSuccess maps booleans to activity status strings', () => {
  assert.equal(statusFromSuccess(true), 'success');
  assert.equal(statusFromSuccess(false), 'failed');
  assert.equal(statusFromSuccess(undefined), 'success');
});

test('coalesceSnapshot keeps prior model/tokens when current event is empty', () => {
  const got = coalesceSnapshot({
    prior: {
      sessionKey: 'agent:main:discord:channel:1',
      model: 'moonshotai/kimi-k2.5',
      tokensIn: 1200,
      tokensOut: 300,
      durationMs: 0,
      thinking: 'high',
      reasoning: true,
      projectTag: 'workspace',
      userId: 'discord:channel:1',
    },
    current: {
      sessionKey: 'agent:main:discord:channel:1',
      model: 'unknown-model',
      tokensIn: 0,
      tokensOut: 0,
      durationMs: 5000,
      thinking: '',
      projectTag: 'workspace',
      userId: 'discord:channel:1',
    },
  });

  assert.equal(got.model, 'moonshotai/kimi-k2.5');
  assert.equal(got.tokensIn, 1200);
  assert.equal(got.tokensOut, 300);
  assert.equal(got.durationMs, 5000);
  assert.equal(got.thinking, 'high');
  assert.equal(got.reasoning, true);
});

test('coalesceSnapshot adopts stronger current usage/model values', () => {
  const got = coalesceSnapshot({
    prior: {
      sessionKey: 'agent:main:main',
      model: 'unknown-model',
      tokensIn: 0,
      tokensOut: 0,
      durationMs: 0,
      projectTag: 'workspace',
      userId: 'unknown-user',
    },
    current: {
      sessionKey: 'agent:main:main',
      model: 'moonshotai/kimi-k2.5',
      tokensIn: 777,
      tokensOut: 55,
      durationMs: 3000,
      projectTag: 'workspace',
      userId: 'telegram:1',
    },
  });

  assert.equal(got.model, 'moonshotai/kimi-k2.5');
  assert.equal(got.tokensIn, 777);
  assert.equal(got.tokensOut, 55);
  assert.equal(got.userId, 'telegram:1');
});

test('settleSnapshot adopts late llm_output snapshot for same session', async () => {
  const recentBySession = new Map();
  const recentByChannel = new Map();

  const current = {
    sessionKey: 'agent:main:main',
    model: 'unknown-model',
    tokensIn: 0,
    tokensOut: 0,
    durationMs: 1000,
    projectTag: 'workspace',
    userId: 'telegram:1',
  };
  recentBySession.set(current.sessionKey, current);
  recentByChannel.set('telegram', current);

  const settled = await settleSnapshot({
    current,
    sessionKey: current.sessionKey,
    channel: 'telegram',
    recentBySession,
    recentByChannel,
    settleMs: 1,
    sleepFn: async () => {
      recentBySession.set(current.sessionKey, {
        ...current,
        model: 'moonshotai/kimi-k2.5',
        tokensIn: 321,
        tokensOut: 44,
      });
    },
  });

  assert.equal(settled.model, 'moonshotai/kimi-k2.5');
  assert.equal(settled.tokensIn, 321);
  assert.equal(settled.tokensOut, 44);
});

test('postWithRetry retries with backoff and fails cleanly after final failure', async () => {
  const payload = { session_key: 's1' };
  const sleeps = [];
  let calls = 0;

  const ok = await postWithRetry({
    payload,
    apiUrl: 'http://localhost:18730/api/activity',
    backoffsMs: [1, 2, 4],
    sleep: async (ms) => sleeps.push(ms),
    postJson: async () => {
      calls += 1;
      throw new Error('boom');
    },
  });

  assert.equal(ok, false);
  assert.equal(calls, 3);
  assert.deepEqual(sleeps, [1, 2]);
});

test('sendToApi queues payload to markdown file after retry exhaustion', async () => {
  const queueRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'clawtivity-plugin-queue-'));
  const payload = { session_key: 'queued-session', model: 'gpt-5' };

  await sendToApi(payload, {
    apiUrl: 'http://localhost:18730/api/activity',
    queueRoot,
    logger: { warn: () => {} },
    postJson: async () => {
      throw new Error('down');
    },
    sleep: async () => {},
    backoffsMs: [1, 2, 4],
  });

  const files = fs.readdirSync(queueRoot).filter((name) => name.endsWith('.md'));
  assert.equal(files.length, 1);

  const body = fs.readFileSync(path.join(queueRoot, files[0]), 'utf8');
  assert.match(body, /"session_key":"queued-session"/);
});

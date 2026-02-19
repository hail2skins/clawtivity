const path = require('node:path');
const os = require('node:os');
const fs = require('node:fs');

const DEFAULT_FRESHNESS_MS = 60_000;
const DEFAULT_BACKOFF_MS = [1000, 2000, 4000];
const DEFAULT_API_URL = 'http://localhost:18730/api/activity';
const DEFAULT_QUEUE_ROOT = path.join(os.homedir(), '.clawtivity', 'queue');

function nowIso() {
  return new Date().toISOString();
}

function asInt(value, fallback = 0) {
  const n = Number(value);
  return Number.isFinite(n) ? Math.round(n) : fallback;
}

function asString(value, fallback = '') {
  if (typeof value === 'string') {
    const trimmed = value.trim();
    return trimmed === '' ? fallback : trimmed;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  return fallback;
}

function shouldUseRecent(recent, now = Date.now(), freshnessMs = DEFAULT_FRESHNESS_MS) {
  if (!recent || typeof recent !== 'object') return false;
  if (!Number.isFinite(recent.ts)) return false;
  return now - recent.ts <= freshnessMs;
}

function channelKeyFromContext(ctx, event) {
  return asString(
    (ctx && (ctx.channelId || ctx.messageProvider || ctx.commandSource))
      || (event && (event.channelId || event.to)),
    'unknown-channel'
  );
}

function sessionKeyFromContext(ctx) {
  return asString(
    (ctx && (ctx.sessionKey || ctx.conversationId || (ctx.session && ctx.session.key))),
    ''
  );
}

function extractUsage(event) {
  const usage = (event && event.usage) || {};
  return {
    tokensIn: asInt(usage.input ?? usage.input_tokens ?? usage.prompt_tokens, 0),
    tokensOut: asInt(usage.output ?? usage.output_tokens ?? usage.completion_tokens, 0),
  };
}

function modelFromEvent(event, ctx) {
  return asString(
    (event && (event.model || (event.result && event.result.model)))
      || (ctx && (ctx.model || (ctx.metadata && ctx.metadata.model))),
    'unknown-model'
  );
}

function statusFromSuccess(success) {
  if (success === false) return 'failed';
  return 'success';
}

function buildActivityPayload(params) {
  const {
    sessionKey,
    model,
    tokensIn,
    tokensOut,
    durationMs,
    projectTag,
    channel,
    userId,
    status,
    toolsUsed,
    nowIso: nowValue,
    fallbackSessionSeed,
  } = params;

  const normalizedChannel = asString(channel, 'unknown-channel');
  const normalizedSeed = asString(fallbackSessionSeed, `unknown:${Date.now()}`);
  const normalizedSession = asString(sessionKey, `channel:${normalizedChannel}:${normalizedSeed}`);

  return {
    session_key: normalizedSession,
    model: asString(model, 'unknown-model'),
    tokens_in: asInt(tokensIn, 0),
    tokens_out: asInt(tokensOut, 0),
    cost_estimate: 0,
    duration_ms: asInt(durationMs, 0),
    project_tag: asString(projectTag, 'unknown-project'),
    external_ref: '',
    category: 'general',
    thinking: 'medium',
    reasoning: false,
    channel: normalizedChannel,
    status: asString(status, 'success'),
    user_id: asString(userId, 'unknown-user'),
    tools_used: Array.isArray(toolsUsed) ? toolsUsed : [],
    created_at: asString(nowValue, nowIso())
  };
}

function mergeRecentByChannel(params) {
  const {
    channelId,
    eventTo,
    conversationId,
    success,
    recent,
    now,
    freshnessMs = DEFAULT_FRESHNESS_MS,
    projectTag,
    userId,
  } = params;

  const useRecent = shouldUseRecent(recent, now, freshnessMs);
  const seed = asString(conversationId, asString(eventTo, 'unknown-target'));

  return buildActivityPayload({
    sessionKey: useRecent ? recent.sessionKey : '',
    model: useRecent ? recent.model : '',
    tokensIn: useRecent ? recent.tokensIn : 0,
    tokensOut: useRecent ? recent.tokensOut : 0,
    durationMs: useRecent ? recent.durationMs : 0,
    projectTag: asString(projectTag, useRecent ? recent.projectTag : path.basename(process.cwd())),
    channel: channelId,
    userId: asString(userId, useRecent ? recent.userId : (conversationId || eventTo || 'unknown-user')),
    status: success ? 'success' : 'failed',
    toolsUsed: [],
    nowIso: nowIso(),
    fallbackSessionSeed: seed,
  });
}

function extractAssistantText(messages) {
  if (!Array.isArray(messages)) return '';
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const entry = messages[i];
    if (!entry || typeof entry !== 'object') continue;
    const role = asString(entry.role, '');
    if (role !== 'assistant') continue;
    if (typeof entry.content === 'string' && entry.content.trim() !== '') {
      return entry.content;
    }
    if (Array.isArray(entry.content)) {
      const textChunk = entry.content.find((c) => c && typeof c === 'object' && c.type === 'text' && typeof c.text === 'string');
      if (textChunk && textChunk.text.trim() !== '') return textChunk.text;
    }
  }
  return '';
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function postJson(url, payload) {
  if (typeof fetch !== 'function') {
    throw new Error('fetch is unavailable in this runtime');
  }

  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }
}

async function postWithRetry(options = {}) {
  const {
    payload,
    apiUrl = DEFAULT_API_URL,
    postJson: postJsonImpl = postJson,
    backoffsMs = DEFAULT_BACKOFF_MS,
    sleep: sleepImpl = sleep,
    logger,
  } = options;

  let lastError;
  for (let i = 0; i < backoffsMs.length; i += 1) {
    try {
      await postJsonImpl(apiUrl, payload);
      return true;
    } catch (err) {
      lastError = err;
      if (i < backoffsMs.length - 1) {
        await sleepImpl(backoffsMs[i]);
      }
    }
  }

  if (logger && typeof logger.warn === 'function') {
    logger.warn(`[clawtivity-activity] failed after retries: ${String(lastError)}`);
  }
  return false;
}

function resolveApiUrl(pluginConfig) {
  return asString(pluginConfig && pluginConfig.apiUrl, DEFAULT_API_URL);
}

function resolveQueueRoot(pluginConfig) {
  return asString(pluginConfig && pluginConfig.queueRoot, DEFAULT_QUEUE_ROOT);
}

function enqueuePayload(queueRoot, payload) {
  fs.mkdirSync(queueRoot, { recursive: true });

  const now = new Date();
  const yyyy = String(now.getFullYear());
  const mm = String(now.getMonth() + 1).padStart(2, '0');
  const dd = String(now.getDate()).padStart(2, '0');
  const filePath = path.join(queueRoot, `${yyyy}-${mm}-${dd}.md`);

  if (!fs.existsSync(filePath)) {
    fs.writeFileSync(filePath, `# Clawtivity Fallback Queue (${yyyy}-${mm}-${dd})\n\n`, 'utf8');
  }

  const block = [
    `## queued_at: ${nowIso()}`,
    '```json',
    JSON.stringify(payload),
    '```',
    '',
  ].join('\n');

  fs.appendFileSync(filePath, block, 'utf8');
}

async function sendToApi(payload, options = {}) {
  const {
    apiUrl = DEFAULT_API_URL,
    queueRoot = DEFAULT_QUEUE_ROOT,
    logger,
    postJson,
    sleep,
    backoffsMs,
  } = options;

  const ok = await postWithRetry({ payload, apiUrl, logger, postJson, sleep, backoffsMs });
  if (!ok && logger && typeof logger.warn === 'function') {
    enqueuePayload(queueRoot, payload);
    logger.warn('[clawtivity-activity] payload queued after retries');
  }
}

module.exports = {
  id: 'clawtivity-activity',
  name: 'Clawtivity Activity Plugin',
  description: 'Logs outbound agent activity using plugin hooks.',
  version: '0.1.0',
  register(api) {
    const pluginConfig = (api && api.pluginConfig) || {};
    const apiUrl = resolveApiUrl(pluginConfig);
    const queueRoot = resolveQueueRoot(pluginConfig);
    const configuredProjectTag = asString(pluginConfig.projectTag, '');
    const configuredUserId = asString(pluginConfig.userId, '');

    const recentByChannel = new Map();
    const userByChannel = new Map();

    api.on('llm_output', (event, ctx) => {
      const channel = channelKeyFromContext(ctx, event);
      const sessionKey = sessionKeyFromContext(ctx);
      if (!sessionKey) return;
      const usage = extractUsage(event);

      recentByChannel.set(channel, {
        ts: Date.now(),
        sessionKey,
        model: modelFromEvent(event, ctx),
        tokensIn: usage.tokensIn,
        tokensOut: usage.tokensOut,
        durationMs: 0,
        projectTag: configuredProjectTag || path.basename(asString(ctx && ctx.workspaceDir, process.cwd())),
        userId: configuredUserId || 'unknown-user',
      });
    });

    api.on('message_received', (event, ctx) => {
      const channel = channelKeyFromContext(ctx, event);
      const from = asString(event && event.from, '');
      if (!from) return;
      userByChannel.set(channel, from);
    });

    api.on('message_sending', (event, ctx) => {
      const channel = channelKeyFromContext(ctx, event);
      const to = asString(event && event.to, '');
      if (!to) return;
      userByChannel.set(channel, to);
    });

    api.on('agent_end', (event, ctx) => {
      const channel = channelKeyFromContext(ctx, event);
      const recent = recentByChannel.get(channel);
      const usage = extractUsage(event);
      const current = recent || {
        ts: Date.now(),
        sessionKey: sessionKeyFromContext(ctx),
        model: modelFromEvent(event, ctx),
        tokensIn: usage.tokensIn,
        tokensOut: usage.tokensOut,
        durationMs: 0,
        projectTag: configuredProjectTag || path.basename(asString(ctx && ctx.workspaceDir, process.cwd())),
        userId: configuredUserId || userByChannel.get(channel) || 'unknown-user',
      };
      current.durationMs = asInt(event && event.durationMs, 0);
      current.ts = Date.now();
      recentByChannel.set(channel, current);

      const payload = buildActivityPayload({
        sessionKey: current.sessionKey,
        model: current.model,
        tokensIn: current.tokensIn,
        tokensOut: current.tokensOut,
        durationMs: current.durationMs,
        projectTag: configuredProjectTag || current.projectTag,
        channel,
        userId: configuredUserId || userByChannel.get(channel) || current.userId,
        status: statusFromSuccess(event && event.success),
        toolsUsed: [],
        nowIso: nowIso(),
        fallbackSessionSeed: `agent-end:${channel}:${Date.now()}`,
      });

      // Touch assistant content so future enhancements can include it.
      extractAssistantText(event && event.messages);
      return sendToApi(payload, { apiUrl, queueRoot, logger: api.logger });
    });
  },

  // exported for unit tests
  buildActivityPayload,
  mergeRecentByChannel,
  shouldUseRecent,
  extractAssistantText,
  channelKeyFromContext,
  extractUsage,
  statusFromSuccess,
  postWithRetry,
  sendToApi,
};

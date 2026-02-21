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

function asBool(value, fallback = undefined) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (normalized === '') return fallback;
    if (['true', '1', 'yes', 'on', 'enabled'].includes(normalized)) return true;
    if (['false', '0', 'no', 'off', 'disabled'].includes(normalized)) return false;
  }
  return fallback;
}

function getByPath(source, dottedPath) {
  if (!source || typeof source !== 'object') return undefined;
  const parts = dottedPath.split('.');
  let current = source;
  for (const part of parts) {
    if (!current || typeof current !== 'object' || !(part in current)) {
      return undefined;
    }
    current = current[part];
  }
  return current;
}

function firstDefined(source, paths) {
  for (const p of paths) {
    const value = getByPath(source, p);
    if (value !== undefined && value !== null) return value;
  }
  return undefined;
}

function normalizeThinking(value) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    if (value <= 1) return 'low';
    if (value >= 3) return 'high';
    return 'medium';
  }

  const normalized = asString(value, '').toLowerCase();
  if (!normalized) return '';
  if (normalized === 'low') return 'low';
  if (normalized === 'medium') return 'medium';
  if (normalized === 'high') return 'high';
  if (['minimal', 'min', 'none', 'off'].includes(normalized)) return 'low';
  if (['default', 'normal', 'std'].includes(normalized)) return 'medium';
  if (normalized.includes('high')) return 'high';
  if (normalized.includes('med')) return 'medium';
  if (normalized.includes('low')) return 'low';
  return '';
}

function normalizeModelRef(value) {
  return asString(value, '').trim().toLowerCase();
}

function modelSupportsReasoning(ref) {
  const modelRef = normalizeModelRef(ref);
  if (!modelRef) return undefined;

  // Provider-qualified ids are authoritative when available.
  if (modelRef === 'nvidia/moonshotai/kimi-k2.5') return true;
  if (modelRef === 'openrouter/moonshotai/kimi-k2.5') return false;

  // Generic model ids where provider is absent are ambiguous; avoid forcing either way.
  return undefined;
}

function extractCognition(event, ctx, prior = {}) {
  const thinkingFromEvent = normalizeThinking(firstDefined(event, [
    'thinking',
    'thinkingLevel',
    'reasoningEffort',
    'reasoning.effort',
    'reasoning.level',
    'settings.reasoningEffort',
    'metadata.thinking',
    'metadata.reasoningEffort',
    'config.reasoningEffort',
    'options.reasoningEffort',
  ]));
  const thinkingFromCtx = normalizeThinking(firstDefined(ctx, [
    'thinking',
    'thinkingLevel',
    'reasoningEffort',
    'reasoning.effort',
    'settings.reasoningEffort',
    'metadata.thinking',
    'metadata.reasoningEffort',
    'modelSettings.reasoningEffort',
    'session.modelSettings.reasoningEffort',
  ]));
  const priorThinking = normalizeThinking(prior && prior.thinking);
  const thinking = thinkingFromEvent || thinkingFromCtx || priorThinking || 'low';

  const explicitReasoning = asBool(
    firstDefined(event, [
      'reasoning.enabled',
      'reasoning',
      'reasoningEnabled',
      'settings.reasoning',
      'settings.reasoningEnabled',
      'metadata.reasoning',
      'modelInfo.reasoning',
      'model.reasoning',
      'agent.modelInfo.reasoning',
      'capabilities.reasoning',
      'config.reasoning',
      'options.reasoning',
      'options.reasoningEnabled',
    ]),
    undefined,
  );
  const explicitReasoningCtx = asBool(
    firstDefined(ctx, [
      'reasoning.enabled',
      'reasoning',
      'reasoningEnabled',
      'settings.reasoning',
      'settings.reasoningEnabled',
      'metadata.reasoning',
      'modelInfo.reasoning',
      'model.reasoning',
      'agent.modelInfo.reasoning',
      'capabilities.reasoning',
      'modelSettings.reasoning',
      'modelSettings.reasoningEnabled',
      'session.modelSettings.reasoning',
      'session.modelSettings.reasoningEnabled',
    ]),
    undefined,
  );
  const reasoningTokens = asInt(firstDefined(event, [
    'usage.reasoning_tokens',
    'usage.reasoningTokens',
    'result.usage.reasoning_tokens',
    'result.usage.reasoningTokens',
  ]), 0);

  const priorReasoning = asBool(prior && prior.reasoning, false);
  const capabilityReasoning = modelSupportsReasoning(firstDefined(event, [
    'modelRef',
    'model_key',
    'modelKey',
    'model',
    'agent.model',
    'metadata.model',
  ]))
    ?? modelSupportsReasoning(firstDefined(ctx, [
      'modelRef',
      'model_key',
      'modelKey',
      'model',
      'agent.model',
      'metadata.model',
    ]));
  let reasoning = explicitReasoning;
  if (reasoning === undefined) reasoning = explicitReasoningCtx;
  if (reasoning === undefined) reasoning = reasoningTokens > 0 ? true : undefined;
  if (reasoning === undefined) reasoning = capabilityReasoning;
  if (reasoning === undefined) reasoning = priorReasoning;

  return {
    thinking,
    reasoning: Boolean(reasoning),
  };
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
    (ctx && (ctx.sessionKey || ctx.conversationId || (ctx.session && ctx.session.key) || ctx.threadId)),
    ''
  );
}

function sessionKeyFromEvent(event) {
  return asString(
    event && (event.sessionKey || event.conversationId || event.threadId || (event.session && event.session.key)),
    '',
  );
}

function extractUsage(event) {
  const usage = (event && (event.usage
    || (event.result && event.result.usage)
    || event.tokenUsage
    || (event.metrics && event.metrics.usage))) || {};
  return {
    tokensIn: asInt(usage.input ?? usage.input_tokens ?? usage.prompt_tokens, 0),
    tokensOut: asInt(usage.output ?? usage.output_tokens ?? usage.completion_tokens, 0),
  };
}

function modelFromEvent(event, ctx) {
  return asString(
    (event && (event.model
      || (event.result && event.result.model)
      || (event.modelInfo && event.modelInfo.id)
      || (event.agent && event.agent.model)
      || (event.metadata && event.metadata.model)))
      || (ctx && (ctx.model
        || (ctx.metadata && ctx.metadata.model)
        || (ctx.agent && ctx.agent.model))),
    'unknown-model'
  );
}

function isKnownModel(model) {
  const normalized = asString(model, 'unknown-model').toLowerCase();
  return normalized !== 'unknown-model';
}

function coalesceSnapshot(params) {
  const { prior, current } = params || {};
  const safePrior = prior || {};
  const safeCurrent = current || {};

  const currentModel = asString(safeCurrent.model, 'unknown-model');
  const priorModel = asString(safePrior.model, 'unknown-model');
  const currentThinking = normalizeThinking(safeCurrent.thinking);
  const priorThinking = normalizeThinking(safePrior.thinking);
  const currentReasoning = asBool(safeCurrent.reasoning, undefined);
  const priorReasoning = asBool(safePrior.reasoning, false);

  return {
    ts: Date.now(),
    sessionKey: asString(safeCurrent.sessionKey, asString(safePrior.sessionKey, '')),
    model: isKnownModel(currentModel) ? currentModel : priorModel,
    tokensIn: Math.max(asInt(safeCurrent.tokensIn, 0), asInt(safePrior.tokensIn, 0)),
    tokensOut: Math.max(asInt(safeCurrent.tokensOut, 0), asInt(safePrior.tokensOut, 0)),
    durationMs: Math.max(asInt(safeCurrent.durationMs, 0), asInt(safePrior.durationMs, 0)),
    thinking: currentThinking || priorThinking || 'low',
    reasoning: currentReasoning === undefined ? priorReasoning : currentReasoning,
    projectTag: asString(safeCurrent.projectTag, asString(safePrior.projectTag, path.basename(process.cwd()))),
    userId: resolveUserId(
      asString(safeCurrent.userId, asString(safePrior.userId, '')),
      asString(safeCurrent.channel, asString(safePrior.channel, 'unknown-channel')),
      asString(safeCurrent.sessionKey, asString(safePrior.sessionKey, '')),
    ),
  };
}

function statusFromSuccess(success) {
  if (success === false) return 'failed';
  return 'success';
}

function resolveUserId(userId, channel, sessionKey) {
  const explicit = asString(userId, '');
  if (explicit) return explicit;
  const normalizedChannel = asString(channel, 'unknown-channel');
  const normalizedSession = asString(sessionKey, '');
  if (normalizedSession) return `${normalizedChannel}:${normalizedSession}`;
  return `${normalizedChannel}:agent:main`;
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
    promptText,
    assistantText,
    thinking,
    reasoning,
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
    thinking: normalizeThinking(thinking) || 'low',
    reasoning: asBool(reasoning, false),
    channel: normalizedChannel,
    status: asString(status, 'success'),
    user_id: resolveUserId(userId, normalizedChannel, normalizedSession),
    tools_used: Array.isArray(toolsUsed) ? toolsUsed : [],
    prompt_text: asString(promptText, ''),
    assistant_text: asString(assistantText, ''),
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
    userId: resolveUserId(
      asString(userId, useRecent ? recent.userId : (conversationId || eventTo || '')),
      channelId,
      useRecent ? recent.sessionKey : asString(conversationId, ''),
    ),
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

function extractUserText(messages) {
  if (!Array.isArray(messages)) return '';
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const entry = messages[i];
    if (!entry || typeof entry !== 'object') continue;
    const role = asString(entry.role, '');
    if (role !== 'user') continue;
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

async function settleSnapshot(options = {}) {
  const {
    current,
    sessionKey,
    channel,
    recentBySession,
    recentByChannel,
    settleMs = 250,
    sleepFn = sleep,
  } = options;

  if (settleMs <= 0) return current;

  await sleepFn(settleMs);

  const lateSession = sessionKey && recentBySession ? recentBySession.get(sessionKey) : null;
  const lateChannel = channel && recentByChannel ? recentByChannel.get(channel) : null;
  const late = lateSession || lateChannel || null;
  if (!late) return current;

  return coalesceSnapshot({
    prior: current,
    current: late,
  });
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

function resolveSettleMs(pluginConfig) {
  return asInt(pluginConfig && pluginConfig.settleMs, 250);
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
    const settleMs = resolveSettleMs(pluginConfig);
    const configuredProjectTag = asString(pluginConfig.projectTag, '');
    const configuredUserId = asString(pluginConfig.userId, '');

    const recentByChannel = new Map();
    const recentBySession = new Map();
    const userByChannel = new Map();

    api.on('llm_output', (event, ctx) => {
      const channel = channelKeyFromContext(ctx, event);
      const sessionKey = asString(sessionKeyFromContext(ctx), sessionKeyFromEvent(event));
      if (!sessionKey) return;
      const usage = extractUsage(event);
      const cognition = extractCognition(event, ctx, recentBySession.get(sessionKey));

      const snapshot = coalesceSnapshot({
        prior: recentBySession.get(sessionKey),
        current: {
          ts: Date.now(),
          sessionKey,
          channel,
          model: modelFromEvent(event, ctx),
          tokensIn: usage.tokensIn,
          tokensOut: usage.tokensOut,
          durationMs: asInt(event && event.durationMs, 0),
          thinking: cognition.thinking,
          reasoning: cognition.reasoning,
          projectTag: configuredProjectTag || path.basename(asString(ctx && ctx.workspaceDir, process.cwd())),
          userId: configuredUserId || userByChannel.get(channel) || '',
        },
      });
      recentBySession.set(sessionKey, snapshot);
      recentByChannel.set(channel, {
        ...snapshot,
        sessionKey,
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

    api.on('agent_end', async (event, ctx) => {
      const channel = channelKeyFromContext(ctx, event);
      const sessionKey = asString(sessionKeyFromContext(ctx), sessionKeyFromEvent(event));
      const recentSession = sessionKey ? recentBySession.get(sessionKey) : null;
      const recentChannel = recentByChannel.get(channel);
      const recent = recentSession || recentChannel || null;
      const usage = extractUsage(event);
      const cognition = extractCognition(event, ctx, recent);
      const current = coalesceSnapshot({
        prior: recent,
        current: {
          ts: Date.now(),
          sessionKey,
          channel,
          model: modelFromEvent(event, ctx),
          tokensIn: usage.tokensIn,
          tokensOut: usage.tokensOut,
          durationMs: asInt(event && event.durationMs, 0),
          thinking: cognition.thinking,
          reasoning: cognition.reasoning,
          projectTag: configuredProjectTag || path.basename(asString(ctx && ctx.workspaceDir, process.cwd())),
          userId: configuredUserId || userByChannel.get(channel) || '',
        },
      });
      if (!current.sessionKey && recentChannel && recentChannel.sessionKey) {
        current.sessionKey = recentChannel.sessionKey;
      }
      current.ts = Date.now();
      if (current.sessionKey) {
        recentBySession.set(current.sessionKey, current);
      }
      recentByChannel.set(channel, current);

      const settled = await settleSnapshot({
        current,
        sessionKey: current.sessionKey,
        channel,
        recentBySession,
        recentByChannel,
        settleMs,
      });
      if (settled.sessionKey) {
        recentBySession.set(settled.sessionKey, settled);
      }
      recentByChannel.set(channel, settled);

      const promptText = extractUserText(event && event.messages);
      const assistantText = extractAssistantText(event && event.messages);
      const payload = buildActivityPayload({
        sessionKey: settled.sessionKey,
        model: settled.model,
        tokensIn: settled.tokensIn,
        tokensOut: settled.tokensOut,
        durationMs: settled.durationMs,
        projectTag: configuredProjectTag || settled.projectTag,
        channel,
        userId: configuredUserId || userByChannel.get(channel) || settled.userId,
        status: statusFromSuccess(event && event.success),
        toolsUsed: [],
        promptText,
        assistantText,
        thinking: settled.thinking,
        reasoning: settled.reasoning,
        nowIso: nowIso(),
        fallbackSessionSeed: `agent-end:${channel}:${Date.now()}`,
      });

      return sendToApi(payload, { apiUrl, queueRoot, logger: api.logger });
    });
  },

  // exported for unit tests
  buildActivityPayload,
  mergeRecentByChannel,
  shouldUseRecent,
  extractAssistantText,
  extractUserText,
  channelKeyFromContext,
  extractUsage,
  resolveUserId,
  extractCognition,
  coalesceSnapshot,
  settleSnapshot,
  statusFromSuccess,
  postWithRetry,
  sendToApi,
};

const path = require('node:path');
const os = require('node:os');
const fs = require('node:fs');

const DEFAULT_FRESHNESS_MS = 60_000;
const DEFAULT_BACKOFF_SECONDS = [1, 2, 4];
const DEFAULT_BACKOFF_MS = DEFAULT_BACKOFF_SECONDS.map((seconds) => Math.round(seconds * 1000));
const DEFAULT_API_URL = 'http://localhost:18730/api/activity';
const DEFAULT_QUEUE_ROOT = path.join(os.homedir(), '.clawtivity', 'queue');
const PROJECT_OVERRIDE_PATTERN = /\bproject\b\s*:?\s*([a-zA-Z0-9][a-zA-Z0-9._-]*)/i;
const PROJECT_PATH_MENTION_PATTERN = /\/projects?\/([a-zA-Z0-9][a-zA-Z0-9._-]*)/i;
const PROJECT_OVERRIDE_STOPWORDS = new Set(['as', 'is', 'was', 'the', 'a', 'an', 'to', 'for']);
const QUEUE_ROOT_ENV = 'CLAWTIVITY_QUEUE_ROOT';
const BACKOFF_SECONDS_ENV = 'CLAWTIVITY_BACKOFF_SECONDS';

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
  if (modelRef === 'moonshotai/kimi-k2.5') return true;

  // Unqualified ids we can safely infer from current deployment conventions.
  return undefined;
}

function normalizeProjectTag(value) {
  const raw = asString(value, '').trim().toLowerCase();
  if (!raw) return '';
  return raw
    .replace(/\s+/g, '-')
    .replace(/[^a-z0-9._-]/g, '')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function trimTrailingProjectPunctuation(value) {
  return asString(value, '').replace(/[.,;:!?)}\]"']+$/g, '');
}

function projectFromPrompt(promptText) {
  const text = asString(promptText, '');
  if (!text) return '';
  const match = text.match(PROJECT_OVERRIDE_PATTERN);
  if (!match || match.length < 2) return '';
  const candidate = normalizeProjectTag(trimTrailingProjectPunctuation(match[1]));
  if (!candidate) return '';
  if (PROJECT_OVERRIDE_STOPWORDS.has(candidate)) return '';
  return candidate;
}

function projectFromPathMention(promptText) {
  const text = asString(promptText, '');
  if (!text) return '';
  const match = text.match(PROJECT_PATH_MENTION_PATTERN);
  if (!match || match.length < 2) return '';
  const candidate = normalizeProjectTag(trimTrailingProjectPunctuation(match[1]));
  if (!candidate) return '';
  return candidate;
}

function projectFromWorkspaceDir(workspaceDir) {
  const dir = asString(workspaceDir, '');
  if (!dir) return '';
  const normalized = dir.replace(/\\/g, '/');
  const match = normalized.match(/\/projects?\/([^/]+)/i);
  if (!match || match.length < 2) return '';
  return normalizeProjectTag(match[1]);
}

function discoverProjectRoots(workspaceDir) {
  const candidates = [asString(workspaceDir, ''), process.cwd()].filter(Boolean);
  const roots = new Set();

  for (const candidate of candidates) {
    const normalized = candidate.replace(/\\/g, '/');
    const match = normalized.match(/^(.*\/projects?)(?:\/.*)?$/i);
    if (match && match[1]) {
      roots.add(path.normalize(match[1]));
    }
    roots.add(path.join(candidate, 'projects'));
    roots.add(path.join(candidate, 'project'));
  }

  return Array.from(roots);
}

function projectExistsUnderKnownRoots(projectTag, workspaceDir) {
  const roots = discoverProjectRoots(workspaceDir);
  if (roots.length === 0) return true;

  for (const root of roots) {
    try {
      const info = fs.statSync(path.join(root, projectTag));
      if (info.isDirectory()) return true;
    } catch (err) {
      // ignore missing roots/directories
    }
  }
  return false;
}

function resolveProjectContext(options = {}) {
  const {
    promptText,
    workspaceDir,
    configuredProjectTag,
  } = options;

  const fromPrompt = projectFromPrompt(promptText);
  if (fromPrompt && projectExistsUnderKnownRoots(fromPrompt, workspaceDir)) {
    return {
      projectTag: fromPrompt,
      projectReason: 'prompt_override',
    };
  }

  const fromPathMention = projectFromPathMention(promptText);
  if (fromPathMention) {
    return {
      projectTag: fromPathMention,
      projectReason: 'prompt_path_mention',
    };
  }

  const fromPath = projectFromWorkspaceDir(workspaceDir);
  if (fromPath) {
    return {
      projectTag: fromPath,
      projectReason: 'workspace_path',
    };
  }

  const fromConfig = normalizeProjectTag(configuredProjectTag);
  if (fromConfig) {
    return {
      projectTag: fromConfig,
      projectReason: 'plugin_config',
    };
  }

  return {
    projectTag: 'workspace',
    projectReason: 'fallback:workspace',
  };
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

  const currentProjectTag = asString(safeCurrent.projectTag, '');
  const priorProjectTag = asString(safePrior.projectTag, 'workspace');
  const currentProjectReason = asString(safeCurrent.projectReason, '');
  const priorProjectReason = asString(safePrior.projectReason, 'fallback:workspace');

  const usePriorProject =
    currentProjectTag === 'workspace'
    && currentProjectReason === 'fallback:workspace'
    && priorProjectTag !== ''
    && priorProjectTag !== 'workspace';

  return {
    ts: Date.now(),
    sessionKey: asString(safeCurrent.sessionKey, asString(safePrior.sessionKey, '')),
    model: isKnownModel(currentModel) ? currentModel : priorModel,
    tokensIn: Math.max(asInt(safeCurrent.tokensIn, 0), asInt(safePrior.tokensIn, 0)),
    tokensOut: Math.max(asInt(safeCurrent.tokensOut, 0), asInt(safePrior.tokensOut, 0)),
    durationMs: Math.max(asInt(safeCurrent.durationMs, 0), asInt(safePrior.durationMs, 0)),
    thinking: currentThinking || priorThinking || 'low',
    reasoning: currentReasoning === undefined ? priorReasoning : currentReasoning,
    projectTag: usePriorProject ? priorProjectTag : asString(currentProjectTag, priorProjectTag),
    projectReason: usePriorProject ? priorProjectReason : asString(currentProjectReason, priorProjectReason),
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
    projectReason,
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
    project_tag: asString(projectTag, 'workspace'),
    project_reason: asString(projectReason, 'fallback:workspace'),
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
    projectTag: asString(projectTag, useRecent ? recent.projectTag : 'workspace'),
    projectReason: useRecent ? asString(recent.projectReason, 'fallback:workspace') : 'fallback:workspace',
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

function extractMessageText(event) {
  if (!event || typeof event !== 'object') return '';
  if (typeof event.text === 'string' && event.text.trim() !== '') return event.text;
  if (typeof event.message === 'string' && event.message.trim() !== '') return event.message;
  if (typeof event.content === 'string' && event.content.trim() !== '') return event.content;
  if (Array.isArray(event.content)) {
    const textChunk = event.content.find((c) => c && typeof c === 'object' && c.type === 'text' && typeof c.text === 'string');
    if (textChunk && textChunk.text.trim() !== '') return textChunk.text;
  }
  return '';
}

function resolvePromptText(options = {}) {
  const { messages, event, cachedPrompt } = options;
  const fromMessages = extractUserText(messages);
  if (fromMessages) return fromMessages;
  const fromEvent = extractMessageText(event);
  if (fromEvent) return fromEvent;
  return asString(cachedPrompt, '');
}

function cacheInboundPrompt(options = {}) {
  const {
    event,
    ctx,
    promptByChannel,
    promptBySession,
  } = options;

  const prompt = extractMessageText(event);
  if (!prompt) return '';

  const channel = channelKeyFromContext(ctx, event);
  if (promptByChannel && typeof promptByChannel.set === 'function') {
    promptByChannel.set(channel, prompt);
  }

  const sessionKey = asString(sessionKeyFromContext(ctx), sessionKeyFromEvent(event));
  if (sessionKey && promptBySession && typeof promptBySession.set === 'function') {
    promptBySession.set(sessionKey, prompt);
  }

  return prompt;
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

function parseSecondsList(value) {
  if (Array.isArray(value)) {
    return value
      .map((entry) => Number(entry))
      .filter((n) => Number.isFinite(n));
  }
  if (typeof value === 'number') {
    return Number.isFinite(value) ? [value] : [];
  }
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return [];
    return trimmed
      .split(',')
      .map((entry) => Number(entry.trim()))
      .filter((n) => Number.isFinite(n));
  }
  return [];
}

function resolveBackoffSeconds(pluginConfig) {
  const envValue = asString(process.env[BACKOFF_SECONDS_ENV], '');
  const envParsed = parseSecondsList(envValue);
  if (envParsed.length > 0) {
    return envParsed;
  }
  const configParsed = parseSecondsList(pluginConfig && pluginConfig.backoffSeconds);
  if (configParsed.length > 0) {
    return configParsed;
  }
  return DEFAULT_BACKOFF_SECONDS;
}

function resolveBackoffMs(pluginConfig) {
  return resolveBackoffSeconds(pluginConfig).map((seconds) => Math.max(0, Math.round(seconds * 1000)));
}

function resolveApiUrl(pluginConfig) {
  return asString(pluginConfig && pluginConfig.apiUrl, DEFAULT_API_URL);
}

function resolveQueueRoot(pluginConfig) {
  const envValue = asString(process.env[QUEUE_ROOT_ENV], '');
  if (envValue) {
    return envValue;
  }
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
    const backoffsMs = resolveBackoffMs(pluginConfig);
    const configuredProjectTag = asString(pluginConfig.projectTag, '');
    const configuredUserId = asString(pluginConfig.userId, '');

    const recentByChannel = new Map();
    const recentBySession = new Map();
    const userByChannel = new Map();
    const promptByChannel = new Map();
    const promptBySession = new Map();

    api.on('llm_output', (event, ctx) => {
      const channel = channelKeyFromContext(ctx, event);
      const sessionKey = asString(sessionKeyFromContext(ctx), sessionKeyFromEvent(event));
      if (!sessionKey) return;
      const usage = extractUsage(event);
      const cognition = extractCognition(event, ctx, recentBySession.get(sessionKey));
      const project = resolveProjectContext({
        workspaceDir: ctx && ctx.workspaceDir,
        configuredProjectTag,
      });

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
          projectTag: project.projectTag,
          projectReason: project.projectReason,
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
      if (from) {
        userByChannel.set(channel, from);
      }
      cacheInboundPrompt({ event, ctx, promptByChannel, promptBySession });
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
      const promptText = resolvePromptText({
        messages: event && event.messages,
        event,
        cachedPrompt: (sessionKey && promptBySession.get(sessionKey)) || promptByChannel.get(channel),
      });
      const assistantText = extractAssistantText(event && event.messages);
      const project = resolveProjectContext({
        promptText,
        workspaceDir: ctx && ctx.workspaceDir,
        configuredProjectTag,
      });
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
          projectTag: project.projectTag,
          projectReason: project.projectReason,
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

      const payload = buildActivityPayload({
        sessionKey: settled.sessionKey,
        model: settled.model,
        tokensIn: settled.tokensIn,
        tokensOut: settled.tokensOut,
        durationMs: settled.durationMs,
        projectTag: settled.projectTag,
        projectReason: settled.projectReason,
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

      return sendToApi(payload, { apiUrl, queueRoot, logger: api.logger, backoffsMs });
    });
  },

  // exported for unit tests
  buildActivityPayload,
  mergeRecentByChannel,
  shouldUseRecent,
  extractAssistantText,
  extractUserText,
  extractMessageText,
  resolvePromptText,
  cacheInboundPrompt,
  channelKeyFromContext,
  extractUsage,
  resolveUserId,
  resolveProjectContext,
  projectFromPrompt,
  projectFromPathMention,
  extractCognition,
  coalesceSnapshot,
  settleSnapshot,
  statusFromSuccess,
  resolveQueueRoot,
  resolveBackoffMs,
  postWithRetry,
  sendToApi,
};

const { spawn } = require('node:child_process');
const os = require('node:os');
const path = require('node:path');
const fs = require('node:fs');

const DEFAULT_FRESHNESS_MS = 60_000;

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

function resolveSkillPath(pluginConfig) {
  return asString(
    pluginConfig && pluginConfig.skillPath,
    path.join(os.homedir(), '.openclaw', 'skills', 'clawtivity', 'scripts', 'log_activity.py')
  );
}

function getPythonCandidates() {
  return ['python3', '/usr/bin/python3', '/opt/homebrew/bin/python3'];
}

function appendPluginError(line) {
  try {
    const dir = path.join(os.homedir(), '.clawtivity');
    fs.mkdirSync(dir, { recursive: true });
    fs.appendFileSync(path.join(dir, 'plugin-errors.log'), `${new Date().toISOString()} ${line}\n`, 'utf8');
  } catch (_) {
    // best effort only
  }
}

function runSkill(pythonBin, skillPath, apiUrl, payload) {
  return new Promise((resolve, reject) => {
    const args = [skillPath];
    if (apiUrl) args.push('--api-url', apiUrl);

    const child = spawn(pythonBin, args, {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: process.env,
    });

    let stderr = '';
    child.stderr.on('data', (chunk) => {
      stderr += String(chunk);
    });

    child.on('error', (err) => reject(err));
    child.on('close', (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`${pythonBin} exited ${code}: ${stderr.trim()}`));
    });

    child.stdin.write(JSON.stringify(payload));
    child.stdin.end();
  });
}

async function sendToSkill(skillPath, apiUrl, payload, logger) {
  let lastError;
  for (const pythonBin of getPythonCandidates()) {
    try {
      await runSkill(pythonBin, skillPath, apiUrl, payload);
      return;
    } catch (err) {
      lastError = err;
    }
  }

  const errorMessage = `[clawtivity-activity] failed to dispatch payload: ${String(lastError)}`;
  appendPluginError(errorMessage);
  if (logger && typeof logger.warn === 'function') {
    logger.warn(errorMessage);
  }
}

module.exports = {
  id: 'clawtivity-activity',
  name: 'Clawtivity Activity Plugin',
  description: 'Logs outbound agent activity using plugin hooks.',
  version: '0.1.0',
  register(api) {
    const pluginConfig = (api && api.pluginConfig) || {};
    const skillPath = resolveSkillPath(pluginConfig);
    const apiUrl = asString(pluginConfig.apiUrl, '');
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
      return sendToSkill(skillPath, apiUrl, payload, api.logger);
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
  getPythonCandidates,
};

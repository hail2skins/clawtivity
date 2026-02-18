const { spawn } = require('node:child_process');
const os = require('node:os');
const path = require('node:path');

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

function sendToSkill(skillPath, apiUrl, payload, logger) {
  return new Promise((resolve, reject) => {
    const args = [skillPath];
    if (apiUrl) args.push('--api-url', apiUrl);

    const child = spawn('python3', args, {
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
      reject(new Error(`clawtivity skill exited ${code}: ${stderr.trim()}`));
    });

    child.stdin.write(JSON.stringify(payload));
    child.stdin.end();
  }).catch((err) => {
    logger.warn(`[clawtivity-activity] failed to dispatch payload: ${String(err)}`);
  });
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
    const freshnessMs = asInt(pluginConfig.freshnessMs, DEFAULT_FRESHNESS_MS);

    const recentByChannel = new Map();

    api.on('llm_output', (event, ctx) => {
      const channel = asString(ctx && ctx.messageProvider, 'unknown-channel');
      const sessionKey = asString(ctx && ctx.sessionKey, '');
      if (!sessionKey) return;

      recentByChannel.set(channel, {
        ts: Date.now(),
        sessionKey,
        model: asString(event && event.model, 'unknown-model'),
        tokensIn: asInt(event && event.usage && event.usage.input, 0),
        tokensOut: asInt(event && event.usage && event.usage.output, 0),
        durationMs: 0,
        projectTag: configuredProjectTag || path.basename(asString(ctx && ctx.workspaceDir, process.cwd())),
        userId: configuredUserId || 'unknown-user',
      });
    });

    api.on('agent_end', (event, ctx) => {
      const channel = asString(ctx && ctx.messageProvider, 'unknown-channel');
      const recent = recentByChannel.get(channel);
      if (recent) {
        recent.durationMs = asInt(event && event.durationMs, 0);
        recent.ts = Date.now();
        recentByChannel.set(channel, recent);
      }

      // Fallback: if turn failed before message delivery, still log activity.
      if (event && event.success === false) {
        const payload = buildActivityPayload({
          sessionKey: asString(ctx && ctx.sessionKey, ''),
          model: recent ? recent.model : 'unknown-model',
          tokensIn: recent ? recent.tokensIn : 0,
          tokensOut: recent ? recent.tokensOut : 0,
          durationMs: asInt(event && event.durationMs, 0),
          projectTag: configuredProjectTag || (recent ? recent.projectTag : path.basename(asString(ctx && ctx.workspaceDir, process.cwd()))),
          channel,
          userId: configuredUserId || (recent ? recent.userId : 'unknown-user'),
          status: 'failed',
          toolsUsed: [],
          nowIso: nowIso(),
          fallbackSessionSeed: `failed:${channel}:${Date.now()}`,
        });
        return sendToSkill(skillPath, apiUrl, payload, api.logger);
      }

      // Touch assistant content so future enhancements can include it.
      extractAssistantText(event && event.messages);
      return undefined;
    });

    api.on('message_sent', (event, ctx) => {
      const now = Date.now();
      const channelId = asString(ctx && ctx.channelId, 'unknown-channel');
      const recent = recentByChannel.get(channelId);

      const payload = mergeRecentByChannel({
        channelId,
        eventTo: asString(event && event.to, ''),
        conversationId: asString(ctx && ctx.conversationId, ''),
        success: Boolean(event && event.success),
        recent,
        now,
        freshnessMs,
        projectTag: configuredProjectTag,
        userId: configuredUserId,
      });

      return sendToSkill(skillPath, apiUrl, payload, api.logger);
    });
  },

  // exported for unit tests
  buildActivityPayload,
  mergeRecentByChannel,
  shouldUseRecent,
  extractAssistantText,
};

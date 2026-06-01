import type {
  ConfiguredProviderViewModel,
  ConversationMessageViewModel,
  ConversationTimelineItemViewModel,
  PermissionModeOptionViewModel,
  PermissionRequestViewModel,
  ProviderModelDiscoveryViewModel,
  ProviderTestViewModel,
  ProviderCatalogItemViewModel,
  ProviderTypeViewModel,
  RuntimeModelOptionViewModel,
  WorkbenchAdapter,
  WorkbenchViewModel,
} from './workbenchTypes.ts';
import { getInitialWorkbenchViewModel, staticWorkbenchAdapter } from './staticWorkbenchAdapter.tsx';

interface RuntimeStatusDTO {
  sessionId?: string;
  workingDir?: string;
  model?: string;
  provider?: string;
  busy?: boolean;
  requests?: {
    activeRequestId?: string;
    running?: number;
  };
}

interface RuntimeSessionDTO {
  id: string;
  title: string;
  updatedAt?: number;
  active?: boolean;
}

interface RuntimeSessionsResponseDTO {
  sessions: RuntimeSessionDTO[];
}

interface RuntimeModelsResponseDTO {
  models: Array<{
    id: string;
    name: string;
    provider: string;
    providerId?: string;
    configuredProviderId?: string;
    configuredProvider?: string;
    selected: boolean;
  }>;
}

interface RuntimeSelectedModelResponseDTO {
  selectedModel: {
    configuredProviderId: string;
    model: string;
  };
  status?: RuntimeStatusDTO;
}

interface RuntimeProviderCatalogResponseDTO {
  providerTypes: ProviderTypeViewModel[];
  providers: ProviderCatalogItemViewModel[];
}

interface RuntimeConfiguredProviderDTO {
  id: string;
  providerId: string;
  name: string;
  remark?: string;
  protocol: string;
  apiEndpoint: string;
  hasApiKey?: boolean;
  proxy?: string;
  defaultModel?: string;
  enabled: boolean;
}

interface RuntimeConfiguredProviderRequestDTO {
  id?: string;
  providerId: string;
  name: string;
  remark?: string;
  protocol: string;
  apiEndpoint?: string;
  apiKey?: string;
  proxy?: string;
  defaultModel?: string;
  enabled: boolean;
}

interface RuntimeConfiguredProvidersResponseDTO {
  providers: RuntimeConfiguredProviderDTO[];
}

interface RuntimeConfiguredProviderResponseDTO {
  provider: RuntimeConfiguredProviderDTO;
}

type RuntimeProviderModelDiscoveryResponseDTO = ProviderModelDiscoveryViewModel;

type RuntimeProviderTestResponseDTO = ProviderTestViewModel;

interface RuntimeChatResponseDTO {
  requestId?: string;
  turnId?: string;
  status: RuntimeStatusDTO;
}

interface RuntimeMessageDTO {
  id: string;
  role: 'user' | 'assistant' | 'tool' | 'system';
  content?: string;
  parts?: Array<{
    type: string;
    text?: string;
    content?: string;
    data?: string;
    thinking?: string;
    startedAt?: number;
    finishedAt?: number;
    message?: string;
    details?: string;
    toolCallId?: string;
    name?: string;
    input?: string;
    isError?: boolean;
  }>;
  provider?: string;
  model?: string;
  createdAt?: number;
  error?: string;
}

interface RuntimeMessagesResponseDTO {
  messages: RuntimeMessageDTO[];
}

interface RuntimeTurnResponseDTO {
  turn: {
    id: string;
    status: string;
    sessionId?: string;
    error?: string;
    latestAssistant?: RuntimeMessageDTO;
  };
}

interface RuntimeTurnDTO {
  id: string;
  sessionId: string;
  status: string;
  startedAt?: number;
  finishedAt?: number;
  error?: string;
}

interface RuntimeToolCallDTO {
  id: string;
  sessionId: string;
  turnId: string;
  name: string;
  source: string;
  command?: string;
  risk?: string;
  policyMode?: string;
  policyReason?: string;
  policyTargetSummary?: string;
  status: string;
  inputSummary?: string;
  outputSummary?: string;
  stdout?: string;
  stderr?: string;
  error?: string;
  startedAt?: number;
  finishedAt?: number;
}

interface RuntimePermissionDTO {
  id: string;
  sessionId: string;
  turnId?: string;
  toolCallId: string;
  toolName: string;
  action: string;
  risk?: string;
  status: string;
  target?: string;
  path?: string;
  reason?: string;
  policyReason?: string;
  policyMode?: string;
  createdAt?: number;
  decidedAt?: number;
}

interface RuntimePolicyDTO {
  mode: string;
  modes?: string[];
  description?: string;
}

interface RuntimePolicyResponseDTO {
  policy: RuntimePolicyDTO;
}

interface RuntimeSessionActivityDTO {
  sessionId: string;
  messages: RuntimeMessageDTO[];
  turns: RuntimeTurnDTO[];
  toolCalls: RuntimeToolCallDTO[];
  permissions: RuntimePermissionDTO[];
  policy: RuntimePolicyDTO;
}

interface RuntimeBridgeModule {
  Status: () => Promise<RuntimeStatusDTO>;
  Sessions: () => Promise<RuntimeSessionsResponseDTO>;
  Models: () => Promise<RuntimeModelsResponseDTO>;
  SelectedModel?: () => Promise<RuntimeSelectedModelResponseDTO>;
  SaveSelectedModel?: (req: { configuredProviderId: string; model: string; scope?: string }) => Promise<RuntimeSelectedModelResponseDTO>;
  ProviderCatalog?: () => Promise<RuntimeProviderCatalogResponseDTO>;
  ConfiguredProviders?: () => Promise<RuntimeConfiguredProvidersResponseDTO>;
  SaveConfiguredProvider?: (req: RuntimeConfiguredProviderRequestDTO) => Promise<RuntimeConfiguredProviderResponseDTO>;
  DeleteConfiguredProvider?: (providerID: string) => Promise<RuntimeConfiguredProvidersResponseDTO>;
  DiscoverConfiguredProviderModels?: (providerID: string) => Promise<RuntimeProviderModelDiscoveryResponseDTO>;
  TestConfiguredProvider?: (providerID: string) => Promise<RuntimeProviderTestResponseDTO>;
  MeasureConfiguredProviderLatency?: (providerID: string) => Promise<RuntimeProviderTestResponseDTO>;
  NewChat: (title: string) => Promise<RuntimeStatusDTO>;
  SelectSession: (sessionID: string) => Promise<RuntimeStatusDTO>;
  DeleteSession?: (sessionID: string) => Promise<RuntimeSessionsResponseDTO>;
  Chat: (req: { prompt: string; sessionId?: string }) => Promise<RuntimeChatResponseDTO>;
  CancelTurn?: (turnID: string) => Promise<RuntimeStatusDTO>;
  Messages?: () => Promise<RuntimeMessagesResponseDTO>;
  SessionMessages?: (sessionID: string) => Promise<RuntimeMessagesResponseDTO>;
  SessionActivity?: (sessionID: string) => Promise<RuntimeSessionActivityDTO>;
  Turn?: (turnID: string) => Promise<RuntimeTurnResponseDTO>;
  Permissions?: () => Promise<{ permissions: RuntimePermissionDTO[] }>;
  GetPolicy?: () => Promise<RuntimePolicyResponseDTO>;
  UpdatePolicy?: (req: { mode: string }) => Promise<RuntimePolicyResponseDTO>;
  DecidePermission?: (req: { permissionId: string; action: string }) => Promise<RuntimeStatusDTO>;
}

let runtimeBridgePromise: Promise<RuntimeBridgeModule | null> | undefined;
const runtimeBridgePath = '/bindings/github.com/charmbracelet/crush/desktop/runtimebridge.js';
const runtimeBridgeTimeoutMS = 750;

function loadRuntimeBridge() {
  if (import.meta.env.DEV && typeof window !== 'undefined') {
    return Promise.resolve(null);
  }

  // Wails generates JavaScript bindings without TypeScript declarations.
  runtimeBridgePromise ??= Promise.race([
    import(
      /* @vite-ignore */
      runtimeBridgePath
    ).then((module) => module as RuntimeBridgeModule),
    new Promise<null>((resolve) => {
      window.setTimeout(() => resolve(null), runtimeBridgeTimeoutMS);
    }),
  ]).catch(() => null);

  return runtimeBridgePromise;
}

function hasProviderSettingsBridge(bridge: RuntimeBridgeModule | null): bridge is RuntimeBridgeModule {
  return Boolean(bridge?.ProviderCatalog && bridge.ConfiguredProviders && bridge.SaveConfiguredProvider && bridge.DeleteConfiguredProvider);
}

function formatUpdatedLabel(updatedAt?: number) {
  if (!updatedAt) {
    return '';
  }

  const normalizedUpdatedAt = normalizeTimestamp(updatedAt);
  const elapsed = Date.now() - normalizedUpdatedAt;
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;

  if (elapsed < 0) {
    return '刚刚';
  }
  if (elapsed < minute) {
    return '刚刚';
  }
  if (elapsed < hour) {
    return `${Math.max(1, Math.floor(elapsed / minute))} 分钟前`;
  }
  if (elapsed < day) {
    return `${Math.floor(elapsed / hour)} 小时前`;
  }

  return `${Math.floor(elapsed / day)} 天前`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}

function modelOptions(modelsResponse?: RuntimeModelsResponseDTO): RuntimeModelOptionViewModel[] {
  const models = Array.isArray(modelsResponse?.models) ? modelsResponse.models : [];
  return models.map((model) => ({
    id: model.id,
    name: model.name || model.id,
    provider: model.provider,
    providerId: model.providerId,
    configuredProviderId: model.configuredProviderId,
    configuredProvider: model.configuredProvider,
    selected: model.selected,
  }));
}

function modelLabel(status?: RuntimeStatusDTO, modelsResponse?: RuntimeModelsResponseDTO) {
  const models = modelOptions(modelsResponse);
  const selectedModel = models.find((model) => model.selected);
  const model = selectedModel?.name || status?.model;

  return model || '未配置模型';
}

function mapSessions(response?: RuntimeSessionsResponseDTO, activeSessionID?: string) {
  const sessions = Array.isArray(response?.sessions) ? response.sessions : [];

  return sessions.map((session) => ({
    id: session.id,
    title: session.title || '新对话',
    updatedLabel: formatUpdatedLabel(session.updatedAt),
    active: session.active || session.id === activeSessionID,
  }));
}

function mapConfiguredProviders(response?: RuntimeConfiguredProvidersResponseDTO): ConfiguredProviderViewModel[] | undefined {
  if (!Array.isArray(response?.providers)) {
    return undefined;
  }

  return response.providers.map((provider) => ({
    id: provider.id,
    providerId: provider.providerId,
    name: provider.name,
    remark: provider.remark,
    apiEndpoint: provider.apiEndpoint,
    protocol: provider.protocol,
    defaultModel: provider.defaultModel,
    tokenConfigured: provider.hasApiKey,
    proxy: provider.proxy,
    enabled: provider.enabled,
  }));
}

function mapConversation(response?: RuntimeMessagesResponseDTO): ConversationMessageViewModel[] {
  const messages = Array.isArray(response?.messages) ? response.messages : [];
  return messages
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .map((message) => ({
      id: message.id,
      role: message.role,
      content: runtimeMessageContent(message),
      createdAt: message.createdAt,
      provider: message.provider,
      model: message.model,
      status: message.error ? 'error' : 'success',
      error: message.error,
    }));
}

const permissionModeOptions: PermissionModeOptionViewModel[] = [
  { value: 'ask', mode: 'ask', label: '\u9ed8\u8ba4\u6a21\u5f0f', description: '\u5de5\u5177\u8c03\u7528\u6309 runtime \u89c4\u5219\u8bf7\u6c42\u5ba1\u6279\u3002' },
  { value: 'auto_read', mode: 'auto_read', label: '\u81ea\u52a8\u5ba1\u67e5', description: '\u53ea\u8bfb\u5de5\u5177\u81ea\u52a8\u6267\u884c\uff0c\u5176\u4f59\u64cd\u4f5c\u8bf7\u6c42\u5ba1\u6279\u3002' },
  {
    value: 'full_access',
    mode: 'full_access',
    label: '\u5b8c\u5168\u8bbf\u95ee\u6743\u9650',
    description: '\u5de5\u5177\u8c03\u7528\u81ea\u52a8\u6267\u884c\uff0c\u4ecd\u53d7 runtime \u5b89\u5168\u8fb9\u754c\u548c\u663e\u5f0f\u62d2\u7edd\u89c4\u5219\u7ea6\u675f\u3002',
  },
];

function permissionMode(policy?: RuntimePolicyDTO) {
  const option = permissionModeOptions.find((item) => item.mode === policy?.mode) ?? permissionModeOptions[0];
  const runtimeDescription = policy?.description?.trim();
  return {
    mode: option.mode,
    label: option.label,
    description: runtimeDescription && !isEnglishRuntimeDescription(runtimeDescription) ? runtimeDescription : option.description,
  };
}

function isEnglishRuntimeDescription(description: string) {
  return [...description].every((char) => char.charCodeAt(0) <= 127);
}

function mapPermission(permission: RuntimePermissionDTO): PermissionRequestViewModel {
  return {
    id: permission.id,
    sessionId: permission.sessionId,
    turnId: permission.turnId,
    toolCallId: permission.toolCallId,
    toolName: permission.toolName,
    action: permission.action,
    risk: permission.risk,
    status: permission.status,
    target: permission.target || permission.path,
    reason: permission.reason || permission.policyReason,
    policyMode: permission.policyMode,
    createdAt: permission.createdAt,
    decidedAt: permission.decidedAt,
  };
}

function mapActivityTimeline(activity?: RuntimeSessionActivityDTO): ConversationTimelineItemViewModel[] {
  if (!activity) {
    return [];
  }
  const messagesDTO = Array.isArray(activity.messages) ? activity.messages : [];
  const toolCallsDTO = Array.isArray(activity.toolCalls) ? activity.toolCalls : [];
  const permissionsDTO = Array.isArray(activity.permissions) ? activity.permissions : [];
  const turnsDTO = Array.isArray(activity.turns) ? activity.turns : [];
  const messages: ConversationTimelineItemViewModel[] = messagesDTO
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .flatMap((message) => {
      const timelineItems: ConversationTimelineItemViewModel[] = [];
      const content = runtimeMessageContent(message);
      if (content.trim() || message.error || message.role === 'user') {
        timelineItems.push({
          id: `message:${message.id}`,
          kind: 'message',
          sessionId: activity.sessionId,
          messageId: message.id,
          role: message.role,
          content,
          status: message.error ? 'error' : 'success',
          createdAt: message.createdAt,
          provider: message.provider,
          model: message.model,
          error: message.error,
        });
      }
      timelineItems.push(...runtimeThinkingItems(message, activity.sessionId));
      return timelineItems;
    });
  const toolCalls: ConversationTimelineItemViewModel[] = toolCallsDTO.map((toolCall) => ({
    id: `tool:${toolCall.id}`,
    kind: 'tool_call',
    sessionId: toolCall.sessionId,
    turnId: toolCall.turnId,
    toolCallId: toolCall.id,
    title: toolCall.name,
    status: toolCall.status,
    summary: toolCall.outputSummary || toolCall.inputSummary,
    createdAt: toolCall.startedAt,
    updatedAt: toolCall.finishedAt,
    error: toolCall.error,
    toolCall: {
      ...toolCall,
    },
  }));
  const permissions: ConversationTimelineItemViewModel[] = permissionsDTO.map((permission) => ({
    id: `permission:${permission.id}`,
    kind: 'permission',
    sessionId: permission.sessionId,
    turnId: permission.turnId,
    toolCallId: permission.toolCallId,
    title: permission.toolName,
    status: permission.status,
    summary: permission.reason || permission.policyReason,
    createdAt: permission.createdAt,
    updatedAt: permission.decidedAt,
    permission: mapPermission(permission),
  }));
  const progress: ConversationTimelineItemViewModel[] = turnsDTO
    .filter((turn) => !['completed'].includes(turn.status))
    .map((turn) => ({
      id: `turn:${turn.id}`,
      kind: 'progress',
      sessionId: turn.sessionId,
      turnId: turn.id,
      title: '运行进度',
      status: turn.status,
      summary: turn.error,
      createdAt: turn.startedAt,
      updatedAt: turn.finishedAt,
      error: turn.error,
    }));

  return [...messages, ...toolCalls, ...permissions, ...progress].sort((left, right) => {
    const leftTime = left.createdAt ?? 0;
    const rightTime = right.createdAt ?? 0;
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function runtimeMessageContent(message: RuntimeMessageDTO) {
  const content = message.content || message.error || '';
  if (content.trim() || !Array.isArray(message.parts)) {
    return content;
  }

  return message.parts
    .map((part) => part.text || part.content || part.data || [part.message, part.details].filter(Boolean).join(': '))
    .filter(Boolean)
    .join('\n\n');
}

function runtimeThinkingItems(message: RuntimeMessageDTO, sessionId: string): ConversationTimelineItemViewModel[] {
  if (!Array.isArray(message.parts)) {
    return [];
  }
  const items: ConversationTimelineItemViewModel[] = [];
  message.parts.forEach((part, index) => {
    if (part.type !== 'reasoning' || !part.thinking?.trim()) {
      return;
    }
    items.push({
        id: `thinking:${message.id}:${index}`,
        kind: 'thinking' as const,
        sessionId,
        messageId: message.id,
        role: message.role,
        title: '运行摘要',
        content: part.thinking.trim(),
        status: part.finishedAt ? 'completed' : 'running',
        createdAt: part.startedAt || message.createdAt,
        updatedAt: part.finishedAt,
        provider: message.provider,
        model: message.model,
    });
  });
  return items;
}

function toConfiguredProviderRequest(provider: ConfiguredProviderViewModel & { token?: string }): RuntimeConfiguredProviderRequestDTO {
  return {
    id: provider.id,
    providerId: provider.providerId,
    name: provider.name,
    remark: provider.remark,
    protocol: provider.protocol || 'openai-compat',
    apiEndpoint: provider.apiEndpoint,
    apiKey: provider.token,
    proxy: provider.proxy,
    defaultModel: provider.defaultModel,
    enabled: provider.enabled ?? true,
  };
}

async function hydrateWorkbench(current: WorkbenchViewModel, bridge: RuntimeBridgeModule) {
  const [status, sessionsResponse, modelsResponse, providerCatalog, configuredProvidersResponse] = await Promise.all([
    optionalRuntimeRequest(() => bridge.Status()),
    optionalRuntimeRequest(() => bridge.Sessions()),
    bridge.Models().catch(() => undefined),
    bridge.ProviderCatalog?.().catch(() => undefined),
    bridge.ConfiguredProviders?.().catch(() => undefined),
  ]);
  const activeSessionID = status?.sessionId || sessionsResponse?.sessions?.find((session) => session.active)?.id;
  const activity = activeSessionID ? await optionalRuntimeRequest(() => bridge.SessionActivity?.(activeSessionID) ?? Promise.resolve(undefined)) : undefined;
  const messagesResponse = activity
    ? { messages: Array.isArray(activity.messages) ? activity.messages : [] }
    : activeSessionID
      ? await optionalRuntimeRequest(() => bridge.SessionMessages?.(activeSessionID) ?? Promise.resolve(undefined))
    : await optionalRuntimeRequest(() => bridge.Messages?.() ?? Promise.resolve(undefined));
  const options = modelOptions(modelsResponse);
  const selectedModel = options.find((model) => model.selected);
  const workingDir = status?.workingDir || current.currentProject.path;
  const busy = typeof status?.busy === 'boolean' ? status.busy : Boolean(current.composer.busy);
  const activeTurnId = status?.requests?.activeRequestId || (busy ? current.composer.activeTurnId : undefined);
  const policy = activity?.policy ?? (await optionalRuntimeRequest(() => bridge.GetPolicy?.() ?? Promise.resolve(undefined)))?.policy;
  const permissions = (Array.isArray(activity?.permissions) ? activity.permissions : []).map(mapPermission);

  return {
    ...current,
    currentProject: {
      ...current.currentProject,
      path: workingDir,
    },
    sessions: mapSessions(sessionsResponse, status?.sessionId),
    conversation: mapConversation(messagesResponse),
    timeline: activity ? mapActivityTimeline(activity) : current.timeline,
    pendingPermissions: permissions.filter((permission) => permission.status === 'pending'),
    composer: {
      ...current.composer,
      permissionLabel: permissionMode(policy).label,
      permissionMode: permissionMode(policy),
      permissionOptions: permissionModeOptions,
      modelLabel: modelLabel(status, modelsResponse),
      selectedModel,
      modelOptions: options,
      busy,
      activeTurnId,
    },
    settings: {
      ...current.settings,
      providerTypes: providerCatalog?.providerTypes ?? current.settings.providerTypes,
      providers: providerCatalog?.providers ?? current.settings.providers,
      configuredProviders: mapConfiguredProviders(configuredProvidersResponse) ?? current.settings.configuredProviders,
    },
  };
}

const runtimeHTTPURL = import.meta.env.DEV ? '/runtime-api' : import.meta.env.VITE_AGENT_BUILDER_RUNTIME_URL || 'http://127.0.0.1:5183';
const runtimeHTTPToken = import.meta.env.VITE_AGENT_BUILDER_RUNTIME_TOKEN || 'agent-builder-dev';
const runtimeOptionalRequestTimeoutMS = 3000;
const runtimeMutationTimeoutMS = 15000;

async function optionalRuntimeRequest<T>(request: () => Promise<T>): Promise<T | undefined> {
  return Promise.race([
    request(),
    new Promise<undefined>((resolve) => {
      window.setTimeout(() => resolve(undefined), runtimeOptionalRequestTimeoutMS);
    }),
  ]).catch(() => undefined);
}

async function runtimeRequestWithTimeout<T>(request: () => Promise<T>, timeoutMS: number, message: string): Promise<T> {
  return Promise.race([
    request(),
    new Promise<never>((_, reject) => {
      window.setTimeout(() => reject(new Error(message)), timeoutMS);
    }),
  ]);
}

interface RuntimeHTTPInit {
  body?: string;
  headers?: Record<string, string>;
  method?: string;
}

async function runtimeFetch<T>(path: string, init?: RuntimeHTTPInit): Promise<T> {
  const url = `${runtimeHTTPURL}${path}`;
  const runtimeGlobal = getRuntimeGlobal();
  const headers = {
    Authorization: `Bearer ${runtimeHTTPToken}`,
    'Content-Type': 'application/json',
    ...init?.headers,
  };

  if (typeof runtimeGlobal?.fetch !== 'function') {
    if (typeof runtimeGlobal?.XMLHttpRequest !== 'function') {
      return runtimeModule<T>(path, init);
    }
    return runtimeXHR<T>(url, {
      body: init?.body,
      headers,
      method: init?.method,
    });
  }

  const response = await runtimeGlobal.fetch(url, {
    body: init?.body,
    headers,
    method: init?.method,
  });
  if (!response.ok) {
    const detail = await runtimeHTTPErrorDetail(response);
    throw new Error(detail || `runtime HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

async function runtimeHTTPErrorDetail(response: Response) {
  try {
    const payload = (await response.json()) as { error?: string };
    return payload.error;
  } catch {
    return '';
  }
}

function getRuntimeGlobal(): (Window & typeof globalThis) | undefined {
  if (typeof globalThis !== 'undefined') {
    return globalThis as Window & typeof globalThis;
  }
  if (typeof window !== 'undefined') {
    return window as Window & typeof globalThis;
  }
  return undefined;
}

async function runtimeModule<T>(path: string, init?: RuntimeHTTPInit): Promise<T> {
  try {
    const params = new URLSearchParams({
      path,
      token: runtimeHTTPToken,
      t: String(Date.now()),
    });
    if (init?.method) {
      params.set('method', init.method);
    }
    if (init?.body) {
      params.set('body', init.body);
    }
    const module = (await import(
      /* @vite-ignore */
      `${runtimeHTTPURL}/v1/dev/module?${params.toString()}`
    )) as { default: T | { error?: string } };
    if (typeof module.default === 'object' && module.default && 'error' in module.default) {
      throw new Error(String(module.default.error));
    }
    return module.default as T;
  } catch (error) {
    console.warn('[runtime] module fallback failed', path, error);
    return runtimeJSONP<T>(path);
  }
}

function runtimeJSONP<T>(path: string): Promise<T> {
  if (typeof document === 'undefined') {
    return Promise.reject(new Error('runtime HTTP request is unavailable'));
  }
  const runtimeGlobal = getRuntimeGlobal();
  if (!runtimeGlobal) {
    return Promise.reject(new Error('runtime HTTP request is unavailable'));
  }
  return new Promise((resolve, reject) => {
    const callback = `__agentBuilderRuntimeJSONP_${Date.now()}_${Math.random().toString(36).slice(2)}`;
    const script = document.createElement('script');
    const cleanup = () => {
      delete (runtimeGlobal as unknown as Record<string, unknown>)[callback];
      if (script.parentNode) {
        script.parentNode.removeChild(script);
      }
    };
    (runtimeGlobal as unknown as Record<string, unknown>)[callback] = (value: T | { error?: string }) => {
      cleanup();
      if (typeof value === 'object' && value && 'error' in value) {
        reject(new Error(String(value.error)));
        return;
      }
      resolve(value as T);
    };
    script.onerror = () => {
      cleanup();
      reject(new Error('runtime HTTP JSONP request failed'));
    };
    script.src = `${runtimeHTTPURL}/v1/dev/jsonp?path=${encodeURIComponent(path)}&token=${encodeURIComponent(runtimeHTTPToken)}&callback=${encodeURIComponent(callback)}`;
    document.head.appendChild(script);
  });
}

function runtimeXHR<T>(
  url: string,
  init: {
    body?: string | null;
    headers: Record<string, string>;
    method?: string;
  },
): Promise<T> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open(init.method ?? 'GET', url, true);
    Object.entries(init.headers).forEach(([key, value]) => request.setRequestHeader(key, value));
    request.onload = () => {
      if (request.status < 200 || request.status >= 300) {
        reject(new Error(`runtime HTTP ${request.status}`));
        return;
      }
      try {
        resolve(JSON.parse(request.responseText) as T);
      } catch (error) {
        reject(error instanceof Error ? error : new Error('failed to decode runtime response'));
      }
    };
    request.onerror = () => reject(new Error('runtime HTTP request failed'));
    request.send(init.body ?? null);
  });
}

const runtimeHTTPBridge: RuntimeBridgeModule = {
  Status: () => runtimeFetch<RuntimeStatusDTO>('/v1/runtime/status'),
  Sessions: () => runtimeFetch<RuntimeSessionsResponseDTO>('/v1/sessions'),
  Models: () => runtimeFetch<RuntimeModelsResponseDTO>('/v1/config/models'),
  SelectedModel: () => runtimeFetch<RuntimeSelectedModelResponseDTO>('/v1/config/selected-model'),
  SaveSelectedModel: (req) =>
    runtimeFetch<RuntimeSelectedModelResponseDTO>('/v1/config/selected-model', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  ProviderCatalog: () => runtimeFetch<RuntimeProviderCatalogResponseDTO>('/v1/config/providers'),
  ConfiguredProviders: () => runtimeFetch<RuntimeConfiguredProvidersResponseDTO>('/v1/config/configured-providers'),
  async SaveConfiguredProvider(req) {
    const method = req.id ? 'PUT' : 'POST';
    const path = req.id ? `/v1/config/configured-providers/${encodeURIComponent(req.id)}` : '/v1/config/configured-providers';
    return runtimeFetch<RuntimeConfiguredProviderResponseDTO>(path, {
      method,
      body: JSON.stringify(req),
    });
  },
  DeleteConfiguredProvider: (providerID) =>
    runtimeFetch<RuntimeConfiguredProvidersResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}`, {
      method: 'DELETE',
    }),
  DiscoverConfiguredProviderModels: (providerID) =>
    runtimeFetch<RuntimeProviderModelDiscoveryResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}/models`, {
      method: 'POST',
    }),
  TestConfiguredProvider: (providerID) =>
    runtimeFetch<RuntimeProviderTestResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}/test`, {
      method: 'POST',
    }),
  MeasureConfiguredProviderLatency: (providerID) =>
    runtimeFetch<RuntimeProviderTestResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}/latency`, {
      method: 'POST',
    }),
  async NewChat(title) {
    return runtimeFetch<RuntimeStatusDTO>('/v1/sessions', {
      method: 'POST',
      body: JSON.stringify({ title }),
    });
  },
  SelectSession: (sessionID) =>
    runtimeFetch<RuntimeStatusDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/select`, {
      method: 'POST',
    }),
  DeleteSession: (sessionID) =>
    runtimeFetch<RuntimeSessionsResponseDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}`, {
      method: 'DELETE',
    }),
  SessionMessages: (sessionID) => runtimeFetch<RuntimeMessagesResponseDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/messages`),
  SessionActivity: (sessionID) => runtimeFetch<RuntimeSessionActivityDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/activity`),
  Turn: (turnID) => runtimeFetch<RuntimeTurnResponseDTO>(`/v1/turns/${encodeURIComponent(turnID)}`),
  Permissions: () => runtimeFetch<{ permissions: RuntimePermissionDTO[] }>('/v1/permissions'),
  GetPolicy: () => runtimeFetch<RuntimePolicyResponseDTO>('/v1/policy'),
  UpdatePolicy: (req) =>
    runtimeFetch<RuntimePolicyResponseDTO>('/v1/policy', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  DecidePermission: (req) =>
    runtimeFetch<RuntimeStatusDTO>(`/v1/permissions/${encodeURIComponent(req.permissionId)}/decision`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  CancelTurn: (turnID) =>
    runtimeFetch<RuntimeStatusDTO>(`/v1/turns/${encodeURIComponent(turnID)}/cancel`, {
      method: 'POST',
    }),
  Chat: (req) => {
    if (req.sessionId) {
      return runtimeFetch<RuntimeChatResponseDTO>(`/v1/sessions/${encodeURIComponent(req.sessionId)}/turns`, {
        method: 'POST',
        body: JSON.stringify(req),
      });
    }
    return runtimeFetch<RuntimeChatResponseDTO>('/v1/turns', {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },
};

async function loadRuntimeHTTPBridge() {
  try {
    await runtimeHTTPBridge.ProviderCatalog?.();
    return runtimeHTTPBridge;
  } catch (error) {
    console.warn('[runtime] provider catalog unavailable', error);
    return null;
  }
}

async function withBridge(
  run: (bridge: RuntimeBridgeModule) => Promise<WorkbenchViewModel>,
  fallback: () => Promise<WorkbenchViewModel>,
) {
  const bridge = await loadRuntimeBridge();
  if (hasProviderSettingsBridge(bridge)) {
    try {
      return await run(bridge);
    } catch (error) {
      console.warn('[runtime] wails bridge failed, trying HTTP bridge', error);
      // Continue to the HTTP runtime bridge below.
    }
  }

  const httpBridge = await loadRuntimeHTTPBridge();
  if (!httpBridge) {
    return fallback();
  }
  try {
    return await run(httpBridge);
  } catch (error) {
    console.warn('[runtime] HTTP bridge failed, using static fallback', error);
    if (httpBridge.ProviderCatalog && httpBridge.ConfiguredProviders) {
      try {
        return await hydrateSettingsOnly(await fallback(), httpBridge);
      } catch (settingsError) {
        console.warn('[runtime] provider settings fallback failed', settingsError);
      }
    }
    return fallback();
  }
}

async function hydrateSettingsOnly(current: WorkbenchViewModel, bridge: RuntimeBridgeModule) {
  const providerCatalog = await bridge.ProviderCatalog?.().catch(() => undefined);
  const configuredProvidersResponse = await bridge.ConfiguredProviders?.().catch(() => undefined);

  return {
    ...current,
    settings: {
      ...current.settings,
      providerTypes: providerCatalog?.providerTypes ?? current.settings.providerTypes,
      providers: providerCatalog?.providers ?? current.settings.providers,
      configuredProviders: mapConfiguredProviders(configuredProvidersResponse) ?? current.settings.configuredProviders,
    },
  };
}

export const wailsWorkbenchAdapter: WorkbenchAdapter = {
  async loadInitialViewModel(mode = 'project') {
    const initial = getInitialWorkbenchViewModel(mode);

    return withBridge(
      (bridge) => hydrateWorkbench(initial, bridge),
      () => staticWorkbenchAdapter.loadInitialViewModel(mode),
    );
  },
  async refresh(current) {
    return withBridge(
      (bridge) => hydrateWorkbench(current, bridge),
      () => staticWorkbenchAdapter.refresh(current),
    );
  },
  async createSession(current) {
    return withBridge(
      async (bridge) => {
        await bridge.NewChat('');
        const hydrated = await hydrateWorkbench({ ...current, mode: 'new-chat', conversation: [], timeline: [] }, bridge);
        return { ...hydrated, mode: 'new-chat' };
      },
      () => staticWorkbenchAdapter.createSession(current),
    );
  },
  async selectSession(current, sessionID) {
    return withBridge(
      async (bridge) => {
        await bridge.SelectSession(sessionID);
        const hydrated = await hydrateWorkbench({ ...current, mode: 'new-chat', conversation: [], timeline: [] }, bridge);
        return { ...hydrated, mode: 'new-chat' };
      },
      () => staticWorkbenchAdapter.selectSession(current, sessionID),
    );
  },
  async deleteSession(current, sessionID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DeleteSession) {
          return staticWorkbenchAdapter.deleteSession(current, sessionID);
        }
        await bridge.DeleteSession(sessionID);
        const wasActive = current.sessions.some((session) => session.id === sessionID && session.active);
        const hydrated = await hydrateWorkbench(
          wasActive ? { ...current, mode: 'new-chat', conversation: [], timeline: [] } : current,
          bridge,
        );
        return { ...hydrated, mode: wasActive ? 'new-chat' : current.mode };
      },
      () => staticWorkbenchAdapter.deleteSession(current, sessionID),
    );
  },
  async selectModel(current, configuredProviderID, model) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SaveSelectedModel) {
          return staticWorkbenchAdapter.selectModel(current, configuredProviderID, model);
        }
        await bridge.SaveSelectedModel({ configuredProviderId: configuredProviderID, model, scope: 'global' });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.selectModel(current, configuredProviderID, model),
    );
  },
  async selectPermissionMode(current, mode) {
    return withBridge(
      async (bridge) => {
        if (!bridge.UpdatePolicy) {
          return current;
        }
        await bridge.UpdatePolicy({ mode });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.selectPermissionMode(current, mode),
    );
  },
  async decidePermission(current, permissionID, action) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DecidePermission) {
          return staticWorkbenchAdapter.decidePermission(current, permissionID, action);
        }
        await bridge.DecidePermission({ permissionId: permissionID, action });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.decidePermission(current, permissionID, action),
    );
  },
  async sendPrompt(current, prompt) {
    const activeSessionID = current.sessions.find((session) => session.active)?.id;

    return withBridge(
      async (bridge) => {
        const existingLoading = current.conversation.findLast((message) => message.status === 'loading' && message.role === 'assistant');
        const hasOptimisticPrompt = current.conversation.some((message) => message.role === 'user' && message.content === prompt);
        const loadingID = existingLoading?.id ?? `loading-${Date.now()}`;
        const optimistic = {
          ...current,
          composer: { ...current.composer, busy: true },
          conversation: current.conversation.some((message) => message.id === loadingID)
            ? current.conversation
            : [
                ...current.conversation,
                ...(hasOptimisticPrompt
                  ? []
                  : [
                      {
                        id: `local-${Date.now()}`,
                        role: 'user' as const,
                        content: prompt,
                        createdAt: Date.now(),
                        status: 'success' as const,
                      },
                    ]),
                {
                  id: loadingID,
                  role: 'assistant' as const,
                  content: '正在生成回复...',
                  status: 'loading' as const,
                },
              ],
        };
        try {
          const response = await runtimeRequestWithTimeout(
            () => bridge.Chat({ prompt, sessionId: activeSessionID }),
            runtimeMutationTimeoutMS,
            '运行时响应超时，请稍后刷新会话查看结果。',
          );
          const responseSessionID = response.status.sessionId || activeSessionID;
          const busyAfterSubmit = Boolean(response.turnId);
          return {
            ...optimistic,
            mode: 'new-chat',
            sessions: responseSessionID
              ? current.sessions.map((session) => ({ ...session, active: session.id === responseSessionID }))
              : current.sessions,
            composer: {
              ...optimistic.composer,
              busy: busyAfterSubmit,
              activeTurnId: busyAfterSubmit ? response.turnId : undefined,
            },
          };
        } catch (error) {
          const message = runtimeErrorMessage(error);
          return {
            ...optimistic,
            mode: 'new-chat',
            conversation: optimistic.conversation.map((item) =>
              item.id === loadingID
                ? {
                    ...item,
                    content: message,
                    status: 'error' as const,
                    error: message,
                  }
                : item,
            ),
            composer: { ...current.composer, busy: false, activeTurnId: undefined },
          };
        }
      },
      () => staticWorkbenchAdapter.sendPrompt(current, prompt),
    );
  },
  async cancelTurn(current, turnID) {
    return withBridge(
      async (bridge) => {
        const targetTurnID = turnID || current.composer.activeTurnId;
        if (targetTurnID && bridge.CancelTurn) {
          await bridge.CancelTurn(targetTurnID);
        }
        const hydrated = await hydrateWorkbench(
          {
            ...current,
            composer: { ...current.composer, busy: false, activeTurnId: undefined },
          },
          bridge,
        );
        return {
          ...hydrated,
          composer: { ...hydrated.composer, busy: false, activeTurnId: undefined },
        };
      },
      () => staticWorkbenchAdapter.cancelTurn(current, turnID),
    );
  },
  async saveConfiguredProvider(current, provider) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SaveConfiguredProvider) {
          return staticWorkbenchAdapter.saveConfiguredProvider(current, provider);
        }
        await bridge.SaveConfiguredProvider(toConfiguredProviderRequest(provider));
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.saveConfiguredProvider(current, provider),
    );
  },
  async deleteConfiguredProvider(current, providerID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DeleteConfiguredProvider) {
          return staticWorkbenchAdapter.deleteConfiguredProvider(current, providerID);
        }
        await bridge.DeleteConfiguredProvider(providerID);
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.deleteConfiguredProvider(current, providerID),
    );
  },
  async discoverConfiguredProviderModels(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.DiscoverConfiguredProviderModels) {
      return bridge.DiscoverConfiguredProviderModels(providerID);
    }
    return runtimeHTTPBridge.DiscoverConfiguredProviderModels?.(providerID) ?? staticWorkbenchAdapter.discoverConfiguredProviderModels(providerID);
  },
  async testConfiguredProvider(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.TestConfiguredProvider) {
      return bridge.TestConfiguredProvider(providerID);
    }
    return runtimeHTTPBridge.TestConfiguredProvider?.(providerID) ?? staticWorkbenchAdapter.testConfiguredProvider(providerID);
  },
  async measureConfiguredProviderLatency(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.MeasureConfiguredProviderLatency) {
      return bridge.MeasureConfiguredProviderLatency(providerID);
    }
    return runtimeHTTPBridge.MeasureConfiguredProviderLatency?.(providerID) ?? staticWorkbenchAdapter.measureConfiguredProviderLatency(providerID);
  },
};

function runtimeErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : '运行时请求失败';
}

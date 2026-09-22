export interface QuickstartConnection {
  id?: string;
  name?: string;
  is_active?: boolean | number;
  health_status?: string;
}

export interface GatewayKey {
  id?: string;
  name?: string;
  key?: string;
  prefix?: string;
  is_active?: boolean;
}

export interface CallableModel {
  id?: string;
  owned_by?: string;
  provider_kind?: string;
  supports_streaming?: boolean;
  connection_id?: string;
  source?: 'discovered' | 'dynamic' | 'catalog' | string;
}

export function maskGatewayKey(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return 'Configured';
  if (trimmed.length <= 8) return `${trimmed.slice(0, 2)}…${trimmed.slice(-2)}`;
  return `${trimmed.slice(0, 4)}…${trimmed.slice(-4)}`;
}

export function recommendCallableModel(models: CallableModel[], connections: QuickstartConnection[] = []): CallableModel | null {
  const activeIDs = new Set(connections.filter(item => item.is_active !== false && item.is_active !== 0).map(item => item.id?.trim()).filter(Boolean));
  if (activeIDs.size === 0) return null;
  const callable = models.filter(model => {
    if (!model.id?.trim()) return false;
    if (model.source === 'catalog') return false;
    return Boolean(model.connection_id && activeIDs.has(model.connection_id));
  });
  if (!callable.length) return null;
  return callable[0];
}

export function buildQuickstartSnippets(input: { baseUrl: string; model: string; gatewayKey: string | null }) {
  const key = input.gatewayKey || 'YOUR_LINTASAN_API_KEY';
  const url = `${input.baseUrl.replace(/\/$/, '')}/chat/completions`;
  const payload = `{"model":"${input.model}","messages":[{"role":"user","content":"Say hello from Lintasan"}]}`;
  return {
    curl: `curl ${url} \\\n  -H "Authorization: Bearer ${key}" \\\n  -H "Content-Type: application/json" \\\n  -d '${payload}'`,
    python: `from openai import OpenAI\n\nclient = OpenAI(base_url="${input.baseUrl}", api_key="${key}")\nresponse = client.chat.completions.create(\n    model="${input.model}",\n    messages=[{"role": "user", "content": "Say hello from Lintasan"}],\n)\nprint(response.choices[0].message.content)`,
    javascript: `import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: "${input.baseUrl}", apiKey: "${key}" });\nconst response = await client.chat.completions.create({\n  model: "${input.model}",\n  messages: [{ role: "user", content: "Say hello from Lintasan" }],\n});\nconsole.log(response.choices[0].message.content);`,
  };
}

export function deriveQuickstart(input: {
  connections: QuickstartConnection[];
  keys: GatewayKey[];
  models: CallableModel[];
  baseUrl: string;
}) {
  const healthyConnections = input.connections.filter(item => item.is_active !== false && item.is_active !== 0 && item.health_status !== 'unhealthy');
  const model = recommendCallableModel(input.models, healthyConnections);
  const connection = model?.connection_id
    ? healthyConnections.find(item => item.id === model.connection_id)
    : healthyConnections[0];
  const key = input.keys.find(item => item.is_active !== false);
  const connectionStep = { ready: Boolean(connection), label: connection?.name || 'Connect a provider account' };
  const keyValue = key?.key || key?.prefix || '';
  const keyStep = { ready: Boolean(key), label: key ? `${key.name || 'Gateway key'} · ${maskGatewayKey(keyValue)}` : 'Create a gateway API key' };
  const modelStep = { ready: Boolean(model), label: model?.id || 'No callable model available yet' };
  const endpointStep = { ready: Boolean(input.baseUrl), label: input.baseUrl };
  const playgroundStep = { ready: Boolean(model), label: model ? 'Ready to test' : 'Choose a callable model first' };
  return {
    connection: connectionStep,
    key: keyStep,
    model: modelStep,
    endpoint: endpointStep,
    playground: playgroundStep,
    recommendedModel: model,
    completedSteps: [connectionStep, keyStep, modelStep, endpointStep, playgroundStep].filter(step => step.ready).length,
  };
}

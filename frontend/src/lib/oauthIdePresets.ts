/** Lab presets: wire connections to OAuth IDE catalog (9router ids). */
export type OAuthIdePreset = {
  oauth_provider: string;
  name: string;
  label: string;
  base_url: string;
  format: string;
  domain: string;
  chatPath?: string;
  modelsPath?: string;
  authHeader?: string;
  authPrefix?: string;
  note?: string;
};

export const OAUTH_IDE_PRESETS: OAuthIdePreset[] = [
  {
    oauth_provider: 'xai',
    name: 'oauth-xai',
    label: 'xAI Grok (OAuth)',
    base_url: 'https://api.x.ai/v1',
    format: 'openai',
    domain: 'x.ai',
    note: 'Authorize in OAuth IDE first'
  },
  {
    oauth_provider: 'claude',
    name: 'oauth-claude',
    label: 'Claude (OAuth)',
    base_url: 'https://api.anthropic.com/v1',
    format: 'anthropic',
    domain: 'anthropic.com',
    chatPath: '/messages',
    authHeader: 'x-api-key',
    authPrefix: '',
    note: 'Subscription OAuth — experimental'
  },
  {
    oauth_provider: 'github',
    name: 'oauth-github-copilot',
    label: 'GitHub Copilot (OAuth)',
    base_url: 'https://api.githubcopilot.com',
    format: 'openai',
    domain: 'github.com',
    note: 'Uses copilot_internal token from device flow'
  },
  {
    oauth_provider: 'codex',
    name: 'oauth-codex',
    label: 'OpenAI Codex (OAuth)',
    base_url: 'https://api.openai.com/v1',
    format: 'openai',
    domain: 'openai.com',
    note: 'Codex subscription OAuth'
  },
  {
    oauth_provider: 'cursor',
    name: 'oauth-cursor',
    label: 'Cursor (import)',
    base_url: 'https://api2.cursor.sh',
    format: 'openai',
    domain: 'cursor.com',
    note: 'Import token on OAuth IDE page first'
  },
  {
    oauth_provider: 'kilocode',
    name: 'oauth-kilocode',
    label: 'Kilo Code (OAuth)',
    base_url: 'https://api.kilo.ai',
    format: 'openai',
    domain: 'kilo.ai'
  },
  {
    oauth_provider: 'cline',
    name: 'oauth-cline',
    label: 'Cline (OAuth)',
    base_url: 'https://api.cline.bot',
    format: 'openai',
    domain: 'cline.bot'
  },
  {
    oauth_provider: 'antigravity',
    name: 'oauth-antigravity',
    label: 'Antigravity (OAuth)',
    base_url: 'https://cloudcode-pa.googleapis.com',
    format: 'openai',
    domain: 'google.com',
    note: 'Google OAuth + Code Assist — env client id required'
  }
];

export function presetByProvider(id: string): OAuthIdePreset | undefined {
  return OAUTH_IDE_PRESETS.find((p) => p.oauth_provider === id);
}
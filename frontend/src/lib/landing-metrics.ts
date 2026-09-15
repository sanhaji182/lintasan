type LandingMetricInput =
  | { status: 'loading' }
  | { status: 'error' }
  | { status: 'ready'; providers: number; models: number };

type LandingMetric = { value: string; label: string };

export function deriveLandingMetrics(input: LandingMetricInput): { kind: 'loading' | 'verified' | 'fallback'; items: LandingMetric[] } {
  if (input.status === 'loading') return { kind: 'loading', items: [] };
  if (input.status === 'ready' && (input.providers > 0 || input.models > 0)) {
    return {
      kind: 'verified',
      items: [
        { value: String(input.providers), label: 'Provider templates' },
        { value: String(input.models), label: 'Catalog models' },
      ],
    };
  }
  return {
    kind: 'fallback',
    items: [
      { value: 'One endpoint', label: 'OpenAI-compatible gateway' },
      { value: 'Failover ready', label: 'Resilient routing' },
      { value: 'Secure', label: 'Keys stay server-side' },
    ],
  };
}

import { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { api, InvocationMetrics } from '../api';

export default function Metrics() {
  const { domainId } = useParams<{ domainId: string }>();
  const [metrics, setMetrics] = useState<InvocationMetrics | null>(null);
  const [prometheus, setPrometheus] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!domainId) return;
    Promise.all([
      api.getMetrics(domainId),
      api.getPrometheus().catch(() => ''),
    ])
      .then(([m, p]) => {
        setMetrics(m);
        setPrometheus(p);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [domainId]);

  if (loading) return <p>Loading metrics...</p>;
  if (error) return <p className="error">{error}</p>;
  if (!metrics) return null;

  return (
    <div>
      <h2>Metrics for {domainId}</h2>
      <div className="actions" style={{ marginBottom: '1rem' }}>
        <Link to="/">← Back</Link>
        <Link to={`/domains/${domainId}`}>Instances</Link>
        <Link to={`/traces/${domainId}`}>Traces</Link>
      </div>

      <div className="metric-grid">
        <div className="metric">
          <div className="value">{metrics.invocation_count}</div>
          <div className="label">Invocations</div>
        </div>
        <div className="metric">
          <div className="value">{metrics.avg_latency_ms.toFixed(2)}ms</div>
          <div className="label">Avg Latency</div>
        </div>
        <div className="metric">
          <div className="value">{metrics.total_prompt_tokens}</div>
          <div className="label">Prompt Tokens</div>
        </div>
        <div className="metric">
          <div className="value">{metrics.total_completion_tokens}</div>
          <div className="label">Completion Tokens</div>
        </div>
        <div className="metric">
          <div className="value">${metrics.total_cost_usd.toFixed(6)}</div>
          <div className="label">Total Cost</div>
        </div>
      </div>

      {prometheus && (
        <div className="card" style={{ marginTop: '1rem' }}>
          <h3>Prometheus</h3>
          <pre>{prometheus}</pre>
        </div>
      )}
    </div>
  );
}

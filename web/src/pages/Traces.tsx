import { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { api, TraceRecord } from '../api';

export default function Traces() {
  const { domainId } = useParams<{ domainId: string }>();
  const [traces, setTraces] = useState<TraceRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!domainId) return;
    api.listTraces(domainId)
      .then(setTraces)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [domainId]);

  if (loading) return <p>Loading traces...</p>;
  if (error) return <p className="error">{error}</p>;

  return (
    <div>
      <h2>Traces for {domainId}</h2>
      <div className="actions" style={{ marginBottom: '1rem' }}>
        <Link to="/">← Back</Link>
        <Link to={`/domains/${domainId}`}>Instances</Link>
        <Link to={`/metrics/${domainId}`}>Metrics</Link>
      </div>
      {traces.length === 0 ? (
        <p>No traces found.</p>
      ) : (
        traces.map((t) => (
          <div key={`${t.session_id}-${t.step_id}-${t.started_at_ms}`} className="card">
            <h3>{t.step_id} <span style={{ color: 'var(--muted)', fontSize: '0.85rem' }}>({t.agent_id})</span></h3>
            <p>Session: {t.session_id} · Duration: {t.duration_ms}ms · Started: {t.started_at_ms}</p>
            <p>Input:</p>
            <pre>{t.input}</pre>
            <p>Output:</p>
            <pre>{t.output}</pre>
          </div>
        ))
      )}
    </div>
  );
}
